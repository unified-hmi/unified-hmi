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

package layoutparams

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/clusterapp"
	"unified-hmi/internal/layout/core"
	_ "unified-hmi/internal/layout/vscreen"
)

func BuildLayoutTreeWoJson(appName, destName string, entry *layoutcore.AppListEntry, vscrnDef *config.VScrnDef) (*layoutcore.LayoutTree, error) {
	dst, err := GetMoveDestRegion(destName)
	if err != nil {
		return nil, fmt.Errorf("buildLayoutTreeWoJson: %w", err)
	}
	vx, vy, vw, vh := dst.VirtualX, dst.VirtualY, dst.VirtualW, dst.VirtualH
	vdisplayID := -1
	cx := vx + vw/2
	cy := vy + vh/2
	for _, display := range vscrnDef.Def2D.VirtualDisplays {
		if cx >= display.VirtualX && cx < display.VirtualX+display.VirtualW &&
			cy >= display.VirtualY && cy < display.VirtualY+display.VirtualH {
			vdisplayID = display.VDisplayId
			break
		}
	}
	appPixelW := float64(entry.AppSize.W)
	appPixelH := float64(entry.AppSize.H)
	for _, display := range vscrnDef.RealDisplays {
		if display.VDisplayId == vdisplayID {
			if appPixelW <= 0 {
				appPixelW = float64(display.PixelW)
			}
			if appPixelH <= 0 {
				appPixelH = float64(display.PixelH)
			}
			break
		}
	}
	layerVID, surfaceVID, err := layoutcore.ComputeAppVIDs(appName)
	if err != nil {
		return nil, err
	}
	visibility := 1
	return &layoutcore.LayoutTree{Vlayers: []layoutcore.VirtualLayer{{
		AppName: appName, VID: layerVID, Coord: layoutcore.COORD_GLOBAL,
		VirtualW: vw, VirtualH: vh, VsrcW: vw, VsrcH: vh,
		VdstX: vx, VdstY: vy, VdstW: vw, VdstH: vh, Visibility: &visibility,
		Vsurfaces: []layoutcore.VirtualSurface{{
			AppName: appName, ParentVID: layerVID, VID: surfaceVID,
			PixelW: appPixelW, PixelH: appPixelH, PsrcW: appPixelW, PsrcH: appPixelH,
			VdstW: vw, VdstH: vh, Visibility: &visibility,
		}},
	}}}, nil
}

var (
	dynamicVIDMu  sync.RWMutex
	dynamicVIDMap = make(map[string]int)
)

// RegisterDynamicVID inserts a (appName, areaName) → VID mapping so that
// GetVIDFromDrawAreas can resolve it without a static config file.
func RegisterDynamicVID(appName, areaName string, vid int) {
	dynamicVIDMu.Lock()
	defer dynamicVIDMu.Unlock()
	dynamicVIDMap[appName+"\x00"+areaName] = vid
}

// UnregisterDynamicVID removes the mapping for (appName, areaName).
func UnregisterDynamicVID(appName, areaName string) {
	dynamicVIDMu.Lock()
	defer dynamicVIDMu.Unlock()
	delete(dynamicVIDMap, appName+"\x00"+areaName)
}

func getDrawIdFromDrawAreas(AppName string, AreaName string) (int, error) {
	drawId := 0
	for _, drawArea := range layoutclusterapp.DrawAreas {
		for _, relation := range drawArea.Relations {
			if drawArea.AppName == AppName && relation.DrawAreaName == AreaName {
				drawId = relation.DrawId
				return drawId, nil
			}
		}
	}

	return drawId, errors.New("Cannot Find Draw Area: " + AppName + ", " + AreaName)
}

func GetVdisplayRegion(dispName string) (layoutcore.VirtualDisplay, error) {
	var vdisplay layoutcore.VirtualDisplay
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return vdisplay, err
	}

	vDisplays := vscrnDef.GetVDisplays()
	for _, vdisp := range vDisplays {
		if vdisp.DispName == dispName {
			vdisplay = vdisp
			return vdisplay, nil
		}
	}

	return vdisplay, errors.New("Cannot Find Display Name from VScrnDef")
}

func GetVDisplayAreaRegion(vdispArea string) (layoutcore.VirtualDisplayArea, error) {
	var area layoutcore.VirtualDisplayArea
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return area, err
	}
	for _, a := range vscrnDef.GetVDisplayAreas() {
		if a.AreaName == vdispArea {
			return a, nil
		}
	}
	return area, errors.New("Cannot Find Display Area Name from VScrnDef")
}

func GetVIDFromDrawAreas(AppName string, AreaName string) (int, error) {
	for _, drawArea := range layoutclusterapp.DrawAreas {
		for _, relation := range drawArea.Relations {
			if drawArea.AppName == AppName && relation.DrawAreaName == AreaName {
				return relation.VLayerId, nil
			}
		}
	}
	dynamicVIDMu.RLock()
	defer dynamicVIDMu.RUnlock()
	if vid, ok := dynamicVIDMap[AppName+"\x00"+AreaName]; ok {
		return vid, nil
	}
	return 0, errors.New("Cannot Find Draw Area: " + AppName + ", " + AreaName)
}

func GetAnimationSetting(AppName string, AreaName string, PatternName string) (layoutcore.AnimationSetting, error) {

	var animationSetting layoutcore.AnimationSetting
	drawId, err := getDrawIdFromDrawAreas(AppName, AreaName)
	if err != nil {
		return animationSetting, err
	}
	for _, as := range layoutclusterapp.AnimationSettings {
		if as.AppName == AppName && as.PatternName == PatternName && as.DrawId == drawId {
			animationSetting = as
			return animationSetting, nil
		}
	}

	return animationSetting, errors.New("Cannot Find Animation Setting: " + AppName + ", " + PatternName)
}

func GetAnimationAngles(vlayer layoutcore.VirtualLayer, timeline []layoutcore.TimeLine, yDown bool) []float64 {
	angles := make([]float64, 0, len(timeline))
	if len(timeline) == 0 {
		return angles
	}

	center := func(x, y, w, h float64) (cx, cy float64) {
		return x + w/2.0, y + h/2.0
	}
	angleDeg := func(ax, ay, bx, by float64) float64 {
		dx := bx - ax
		dy := by - ay
		if yDown {
			dy = -dy
		}
		rad := math.Atan2(dy, dx)
		deg := rad * 180.0 / math.Pi
		deg = math.Mod(deg+360.0, 360.0)
		if deg == 360.0 {
			deg = 0.0
		}
		return math.Round(deg*10) / 10
	}

	ax, ay := center(
		float64(vlayer.VdstX),
		float64(vlayer.VdstY),
		float64(vlayer.VdstW),
		float64(vlayer.VdstH),
	)
	bx, by := center(
		float64(timeline[0].VdstX),
		float64(timeline[0].VdstY),
		float64(timeline[0].VdstW),
		float64(timeline[0].VdstH),
	)
	angles = append(angles, angleDeg(ax, ay, bx, by))

	for i := 1; i < len(timeline); i++ {
		ax, ay = center(
			float64(timeline[i-1].VdstX),
			float64(timeline[i-1].VdstY),
			float64(timeline[i-1].VdstW),
			float64(timeline[i-1].VdstH),
		)
		bx, by = center(
			float64(timeline[i].VdstX),
			float64(timeline[i].VdstY),
			float64(timeline[i].VdstW),
			float64(timeline[i].VdstH),
		)
		angles = append(angles, angleDeg(ax, ay, bx, by))
	}
	return angles
}

func GetTimeFrames(timeline []layoutcore.TimeLine) []float64 {
	frames := make([]float64, 0, len(timeline))
	for _, tl := range timeline {
		frames = append(frames, tl.TimeFrame)
	}
	return frames
}

func GenerateJsonStringFromMap(m map[string]interface{}) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func resolveBuiltinSubArea(destName string, vDisplays []layoutcore.VirtualDisplay) (layoutcore.VirtualDisplayArea, bool) {
	for _, d := range vDisplays {
		prefix := d.DispName + "_"
		if !strings.HasPrefix(destName, prefix) {
			continue
		}
		suffix := destName[len(prefix):]
		var vx, vy, vw, vh float64
		switch suffix {
		case "TOP":
			vx, vy, vw, vh = d.VirtualX, d.VirtualY, d.VirtualW, d.VirtualH/2
		case "BOTTOM":
			vx, vy, vw, vh = d.VirtualX, d.VirtualY+d.VirtualH/2, d.VirtualW, d.VirtualH/2
		case "LEFT":
			vx, vy, vw, vh = d.VirtualX, d.VirtualY, d.VirtualW/2, d.VirtualH
		case "RIGHT":
			vx, vy, vw, vh = d.VirtualX+d.VirtualW/2, d.VirtualY, d.VirtualW/2, d.VirtualH
		case "TL":
			vx, vy, vw, vh = d.VirtualX, d.VirtualY, d.VirtualW/2, d.VirtualH/2
		case "TR":
			vx, vy, vw, vh = d.VirtualX+d.VirtualW/2, d.VirtualY, d.VirtualW/2, d.VirtualH/2
		case "BL":
			vx, vy, vw, vh = d.VirtualX, d.VirtualY+d.VirtualH/2, d.VirtualW/2, d.VirtualH/2
		case "BR":
			vx, vy, vw, vh = d.VirtualX+d.VirtualW/2, d.VirtualY+d.VirtualH/2, d.VirtualW/2, d.VirtualH/2
		default:
			continue
		}
		return layoutcore.VirtualDisplayArea{
			AreaName: destName,
			VirtualX: vx,
			VirtualY: vy,
			VirtualW: vw,
			VirtualH: vh,
		}, true
	}
	return layoutcore.VirtualDisplayArea{}, false
}

// GetMoveDestRegion resolves the destination coordinates for MoveWindow
// and LaunchApp.  Lookup is performed in three tiers:
//  1. virtual_displays.disp_name  (exact match)
//  2. virtual_display_areas.area_name  (JSON-defined named areas)
//  3. Built-in sub-areas: "{dispName}_{SUFFIX}" computed on the fly from
//     each virtual_display.  Supported suffixes:
//     TOP, BOTTOM, LEFT, RIGHT — halves; TL, TR, BL, BR — quarters.
func GetMoveDestRegion(destName string) (layoutcore.VirtualDisplayArea, error) {
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return layoutcore.VirtualDisplayArea{}, err
	}
	vDisplays := vscrnDef.GetVDisplays()
	for _, d := range vDisplays {
		if d.DispName == destName {
			return layoutcore.VirtualDisplayArea{
				AreaName: d.DispName,
				VirtualX: d.VirtualX,
				VirtualY: d.VirtualY,
				VirtualW: d.VirtualW,
				VirtualH: d.VirtualH,
			}, nil
		}
	}
	for _, a := range vscrnDef.GetVDisplayAreas() {
		if a.AreaName == destName {
			return a, nil
		}
	}
	if area, ok := resolveBuiltinSubArea(destName, vDisplays); ok {
		return area, nil
	}
	return layoutcore.VirtualDisplayArea{}, errors.New(
		"Cannot find dest_name in virtual_displays, virtual_display_areas, or built-in sub-areas: " + destName,
	)
}

// MoveDestEntry is a single destination returned by ListAllMoveDestinations.
type MoveDestEntry struct {
	Area layoutcore.VirtualDisplayArea
	Tier int // 1=virtual_display, 2=virtual_display_area, 3=built-in sub-area
}

// ListAllMoveDestinations enumerates every valid dest_name value across the
// three lookup tiers (Tier1 first; on name conflicts the lower tier wins).
func ListAllMoveDestinations() ([]MoveDestEntry, error) {
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return nil, err
	}
	vDisplays := vscrnDef.GetVDisplays()

	seen := make(map[string]bool)
	var result []MoveDestEntry

	for _, d := range vDisplays {
		if seen[d.DispName] {
			continue
		}
		seen[d.DispName] = true
		result = append(result, MoveDestEntry{
			Tier: 1,
			Area: layoutcore.VirtualDisplayArea{
				AreaName: d.DispName,
				VirtualX: d.VirtualX,
				VirtualY: d.VirtualY,
				VirtualW: d.VirtualW,
				VirtualH: d.VirtualH,
			},
		})
	}

	for _, a := range vscrnDef.GetVDisplayAreas() {
		if seen[a.AreaName] {
			continue
		}
		seen[a.AreaName] = true
		result = append(result, MoveDestEntry{Tier: 2, Area: a})
	}

	suffixes := []string{"TOP", "BOTTOM", "LEFT", "RIGHT", "TL", "TR", "BL", "BR"}
	for _, d := range vDisplays {
		for _, sfx := range suffixes {
			name := d.DispName + "_" + sfx
			if seen[name] {
				continue
			}
			area, ok := resolveBuiltinSubArea(name, vDisplays)
			if !ok {
				continue
			}
			seen[name] = true
			result = append(result, MoveDestEntry{Tier: 3, Area: area})
		}
	}

	return result, nil
}

// GetWidenedRegion resolves two virtual displays by name, verifies that they
// share an edge (adjacency: shared edge length > 0), and returns the
// axis-aligned bounding box that covers both displays as a VirtualDisplayArea.
// dispName1 and dispName2 must both be present in virtual_displays.disp_name.
// Returns an error when either display is not found or the displays are not
// adjacent (corner-only contact is not considered adjacent).
func GetWidenedRegion(dispName1, dispName2 string) (layoutcore.VirtualDisplayArea, error) {
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return layoutcore.VirtualDisplayArea{}, err
	}

	var d1, d2 layoutcore.VirtualDisplay
	found1, found2 := false, false
	for _, d := range vscrnDef.GetVDisplays() {
		if d.DispName == dispName1 {
			d1 = d
			found1 = true
		}
		if d.DispName == dispName2 {
			d2 = d
			found2 = true
		}
	}
	if !found1 {
		return layoutcore.VirtualDisplayArea{}, errors.New("display not found in virtual_displays: " + dispName1)
	}
	if !found2 {
		return layoutcore.VirtualDisplayArea{}, errors.New("display not found in virtual_displays: " + dispName2)
	}

	const eps = 0.001
	adjacent := false
	overlapY := math.Min(d1.VirtualY+d1.VirtualH, d2.VirtualY+d2.VirtualH) - math.Max(d1.VirtualY, d2.VirtualY)
	if overlapY > eps {
		if math.Abs(d1.VirtualX+d1.VirtualW-d2.VirtualX) < eps ||
			math.Abs(d2.VirtualX+d2.VirtualW-d1.VirtualX) < eps {
			adjacent = true
		}
	}
	overlapX := math.Min(d1.VirtualX+d1.VirtualW, d2.VirtualX+d2.VirtualW) - math.Max(d1.VirtualX, d2.VirtualX)
	if overlapX > eps {
		if math.Abs(d1.VirtualY+d1.VirtualH-d2.VirtualY) < eps ||
			math.Abs(d2.VirtualY+d2.VirtualH-d1.VirtualY) < eps {
			adjacent = true
		}
	}
	if !adjacent {
		return layoutcore.VirtualDisplayArea{}, errors.New("displays are not adjacent: " + dispName1 + " and " + dispName2)
	}

	x := math.Min(d1.VirtualX, d2.VirtualX)
	y := math.Min(d1.VirtualY, d2.VirtualY)
	w := math.Max(d1.VirtualX+d1.VirtualW, d2.VirtualX+d2.VirtualW) - x
	h := math.Max(d1.VirtualY+d1.VirtualH, d2.VirtualY+d2.VirtualH) - y

	return layoutcore.VirtualDisplayArea{
		AreaName: dispName1 + "+" + dispName2,
		VirtualX: x,
		VirtualY: y,
		VirtualW: w,
		VirtualH: h,
	}, nil
}
