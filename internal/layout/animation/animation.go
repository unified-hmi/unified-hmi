// SPDX-License-Identifier: Apache-2.0
/**
 * Copyright (c) 2026  Panasonic Automotive Systems, Co., Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package layoutanimation

import (
	"math"
	"reflect"
	"sync"
	"time"
	"unified-hmi/internal/layout/commgen"
	"unified-hmi/internal/layout/core"
	"unified-hmi/internal/layout/multiconn"
	"unified-hmi/internal/layout/params"
	"unified-hmi/internal/layout/vscreen"
	. "unified-hmi/internal/ulog"
	"unsafe"
)

var HandleMap map[unsafe.Pointer]*sync.WaitGroup = make(map[unsafe.Pointer]*sync.WaitGroup)

func calcCubicBezierValue(x0 float64, x1 float64, x2 float64, x3 float64, t float64) float64 {
	var a, b, c, t2, t3 float64
	t2 = t * t
	t3 = t2 * t

	c = 3.0 * (x1 - x0)
	b = 3.0*(x2-x1) - c
	a = x3 - x0 - c - b

	return a*t3 + b*t2 + c*t + x0
}

func calcCubicBezierTime(x0 float64, x1 float64, x2 float64, x3 float64, time float64, t0 *float64, t1 *float64) float64 {
	var v, t float64
	t = *t0 + (*t1-*t0)*0.5
	v = calcCubicBezierValue(x0, x1, x2, x3, t)

	if math.Abs(time-v) > 0.001 {
		if v > time {
			*t1 = t
		} else {
			*t0 = t
		}
		return calcCubicBezierTime(x0, x1, x2, x3, time, t0, t1)
	} else {
		return t
	}
}

func calcCubicBezier(keyX [4]float64, keyY [4]float64, time float64) float64 {
	var t, t0, t1 float64
	t = 0.0
	t0 = 0.0
	t1 = 1.0
	var val float64
	if time <= keyX[0] {
		return keyY[0]
	}

	if time >= keyX[3] {
		return keyY[3]
	}

	t = calcCubicBezierTime(keyX[0], keyX[1], keyX[2], keyX[3], time, &t0, &t1)
	val = calcCubicBezierValue(keyY[0], keyY[1], keyY[2], keyY[3], t)

	return val
}

func calcParamsFromBezier(sx float64, sy float64, sw float64, sh float64, dx float64, dy float64, dw float64, dh float64, cubic_param [4]float64, anim_duration float64, elapsed_ms float64) (float64, float64, float64, float64) {
	var anim_key_x [4]float64
	anim_key_x[0] = 0.0
	anim_key_x[1] = anim_duration * cubic_param[0]
	anim_key_x[2] = anim_duration * cubic_param[2]
	anim_key_x[3] = anim_duration

	var anim_key_y [4]float64
	anim_key_y[0] = 0.0
	anim_key_y[1] = cubic_param[1]
	anim_key_y[2] = cubic_param[3]
	anim_key_y[3] = 1.0

	loop_num := int(elapsed_ms) / int(anim_duration)
	anim_time := elapsed_ms - (float64(loop_num) * anim_duration)

	ratio := calcCubicBezier(anim_key_x, anim_key_y, anim_time)

	if loop_num%2 > 0 {
		ratio = 1.0 - ratio
	}

	x := layoutcore.RoundTo5(sx + (dx-sx)*ratio)
	y := layoutcore.RoundTo5(sy + (dy-sy)*ratio)
	w := layoutcore.RoundTo5(sw + (dw-sw)*ratio)
	h := layoutcore.RoundTo5(sh + (dh-sh)*ratio)

	return x, y, w, h
}

// CubicParamFromCurveID maps a curve ID to its cubic-bezier control points.
func CubicParamFromCurveID(curveId int) [4]float64 {
	switch curveId {
	case 0:
		return [4]float64{0.42, 0.0, 1.0, 1.0}
	case 1:
		return [4]float64{0.0, 0.0, 0.58, 1.0}
	case 2:
		return [4]float64{0.42, 0.0, 0.58, 1.0}
	case 3:
		return [4]float64{0.0, 0.0, 1.0, 1.0}
	case 4:
		return [4]float64{0.5, 1.5, 0.8, 1.0}
	default:
		return [4]float64{0.0, 0.0, 1.0, 1.0}
	}
}

func StartMove(srcVlayer layoutcore.VirtualLayer, dstVlayer layoutcore.VirtualLayer, cubicParam [4]float64, durationMs float64, wg *sync.WaitGroup) {

	defer func() {
		if wg != nil {
			wg.Done()
			for handle, animationWg := range HandleMap {
				if animationWg == wg {
					delete(HandleMap, handle)
				}
			}
		}
	}()

	var tmpVlayer layoutcore.VirtualLayer
	nextVlayer := srcVlayer
	startTime := time.Now()

	for {
		loop_start_time := time.Now()
		timeFlag := 0
		elapsedMs := float64(time.Since(startTime) / time.Millisecond)

		if elapsedMs >= durationMs {
			nextVlayer = dstVlayer
			timeFlag = 1
		} else {

			dstX, dstY, dstW, dstH := calcParamsFromBezier(
				srcVlayer.VdstX,
				srcVlayer.VdstY,
				srcVlayer.VdstW,
				srcVlayer.VdstH,
				dstVlayer.VdstX,
				dstVlayer.VdstY,
				dstVlayer.VdstW,
				dstVlayer.VdstH,
				cubicParam,
				durationMs,
				elapsedMs)

			srcX, srcY, srcW, srcH := calcParamsFromBezier(
				srcVlayer.VsrcX,
				srcVlayer.VsrcY,
				srcVlayer.VsrcW,
				srcVlayer.VsrcH,
				dstVlayer.VsrcX,
				dstVlayer.VsrcY,
				dstVlayer.VsrcW,
				dstVlayer.VsrcH,
				cubicParam,
				durationMs,
				elapsedMs)

			nextVlayer.VdstX = dstX
			nextVlayer.VdstY = dstY
			nextVlayer.VdstW = dstW
			nextVlayer.VdstH = dstH
			nextVlayer.VsrcX = srcX
			nextVlayer.VsrcY = srcY
			nextVlayer.VsrcW = srcW
			nextVlayer.VsrcH = srcH

		}

		if !reflect.DeepEqual(nextVlayer, tmpVlayer) {
			tmpVlayer = nextVlayer

			loop_elapsed_us := float64(time.Since(loop_start_time) / time.Microsecond)
			sync_wait_us := 16600 - loop_elapsed_us
			if sync_wait_us > 0 {
				time.Sleep(time.Microsecond * time.Duration(sync_wait_us))
			}

			var layoutComm string
			var err error
			layoutComm, err = layoutcommgen.GenerateCommModifyVlayer(nextVlayer)
			if err != nil {
				ELog.Println(err)
				return
			}

			err = layoutmulticonn.MulCon.SendLayoutCommand(layoutComm)
			if err != nil {
				ELog.Println(err)
				return
			}
		}

		if timeFlag == 1 {
			break
		}
	}
}

type WindowOrderHandler interface {
	// ApplyWindowOrder applies a window_order setting for the given VID.
	// If windowOrder is nil, the layer should be restored to its original position.
	ApplyWindowOrder(vid int, appName string, areaName string, windowOrder *layoutcore.WindowOrderSetting) error
}

func StartAnimation(srcVlayer layoutcore.VirtualLayer, as layoutcore.AnimationSetting, areaName string, orderHandler WindowOrderHandler, wg *sync.WaitGroup) {

	defer func() {
		if wg != nil {
			wg.Done()
			for handle, animationWg := range HandleMap {
				if animationWg == wg {
					delete(HandleMap, handle)
				}
			}
		}
	}()

	dstVlayer := srcVlayer
	for _, tl := range as.TimeLine {
		if orderHandler != nil {
			err := orderHandler.ApplyWindowOrder(srcVlayer.VID, as.AppName, areaName, tl.WindowOrder)
			if err != nil {
				WLog.Printf("Window order change failed for VID=%d: %v", srcVlayer.VID, err)
			}
		}

		cubic_params := [4]float64{tl.Curve.X0, tl.Curve.Y0, tl.Curve.X1, tl.Curve.Y1}
		dstVlayer.VdstX = tl.VdstX
		dstVlayer.VdstY = tl.VdstY
		dstVlayer.VdstW = tl.VdstW
		dstVlayer.VdstH = tl.VdstH
		dstVlayer.VsrcX = tl.VsrcX
		dstVlayer.VsrcY = tl.VsrcY
		dstVlayer.VsrcW = tl.VsrcW
		dstVlayer.VsrcH = tl.VsrcH
		StartMove(srcVlayer, dstVlayer, cubic_params, tl.TimeFrame, nil)
		srcVlayer = dstVlayer
	}
}

// GenerateWoJsonMoveParams builds the move parameters for an app launched via
// LaunchApp. The destination is resolved by destName via GetMoveDestRegion's
// 3-tier lookup: virtual_displays.disp_name first, then
// virtual_display_areas.area_name, then built-in sub-areas
// {dispName}_{TOP|BOTTOM|LEFT|RIGHT|TL|TR|BL|BR} as final fallback.
// The source layer VID is derived from the same hash as the app launch
// (layoutcore.ComputeAppLayerVID), keeping move and launch consistent.
func GenerateWoJsonMoveParams(appName string, destName string, curveId int) (*layoutcore.VirtualLayer, *layoutcore.VirtualLayer, *[4]float64, error) {
	dst, err := layoutparams.GetMoveDestRegion(destName)
	if err != nil {
		ELog.Println(err)
		return nil, nil, nil, err
	}

	vid, err := layoutcore.ResolveAppLayerVID(appName)
	if err != nil {
		ELog.Println(err)
		return nil, nil, nil, err
	}

	srcVlayer, err := layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		ELog.Println(err)
		return nil, nil, nil, err
	}

	cubic_param := CubicParamFromCurveID(curveId)

	dstVlayer := srcVlayer
	if srcVlayer.Coord == layoutcore.COORD_GLOBAL {
		dstVlayer.VdstX = dst.VirtualX
		dstVlayer.VdstY = dst.VirtualY
		dstVlayer.VdstW = dst.VirtualW
		dstVlayer.VdstH = dst.VirtualH
	}

	return &srcVlayer, &dstVlayer, &cubic_param, nil
}
