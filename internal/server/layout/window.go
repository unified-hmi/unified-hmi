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

package layoutserver

import (
	"context"
	"fmt"
	"sync"

	layoutanimation "unified-hmi/internal/layout/animation"
	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcommgen "unified-hmi/internal/layout/commgen"
	layoutcore "unified-hmi/internal/layout/core"
	layoutmulticonn "unified-hmi/internal/layout/multiconn"
	layoutparams "unified-hmi/internal/layout/params"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

// MoveWindow moves the window of an app launched via LaunchApp to the
// destination resolved from dest_name via GetMoveDestRegion's 3-tier lookup:
// virtual_displays.disp_name first, then virtual_display_areas.area_name, then
// built-in sub-areas {dispName}_{TOP|BOTTOM|LEFT|RIGHT|TL|TR|BL|BR} as final
// fallback.
func (s *Server) MoveWindow(ctx context.Context, req *uhmi.MoveWindowRequest) (*uhmi.MoveWindowResponse, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	destName := req.GetDestName()
	durationMs := req.GetDurationMs()
	curveId := int(req.GetCurveId())

	srcVlayer, dstVlayer, cubicParam, err := layoutanimation.GenerateWoJsonMoveParams(appName, destName, curveId)
	if err != nil {
		return &uhmi.MoveWindowResponse{Status: "Failed to MoveWindow", Info: err.Error()}, err
	}

	if rerr := s.reorderBeforeMove(srcVlayer.VID, appName, *dstVlayer); rerr != nil {
		WLog.Printf("[MoveWindow] reorderBeforeMove: %v", rerr)
	}
	layoutanimation.StartMove(*srcVlayer, *dstVlayer, *cubicParam, durationMs, nil)
	return &uhmi.MoveWindowResponse{Status: "Window moved successfully"}, nil
}

// MoveWindows moves multiple app windows simultaneously.
// All move parameters are pre-computed for every window first; then all
// StartMove goroutines are released at once via a shared ready channel so that
// animation loops begin as close together in time as possible.
// Results are collected per-window (best-effort).
func (s *Server) MoveWindows(ctx context.Context, req *uhmi.MoveWindowsRequest) (*uhmi.MoveWindowsResponse, error) {
	serverutil.LogFunc()
	windows := req.GetWindows()
	if len(windows) == 0 {
		return &uhmi.MoveWindowsResponse{Status: "error", Info: "no windows specified"}, nil
	}

	type moveParams struct {
		appName    string
		src        layoutcore.VirtualLayer
		dst        layoutcore.VirtualLayer
		cubic      [4]float64
		durationMs float64
		prepErr    string
	}

	params := make([]moveParams, len(windows))
	for i, w := range windows {
		appName := w.GetAppName()
		destName := w.GetDestName()
		durationMs := w.GetDurationMs()
		curveId := int(w.GetCurveId())
		src, dst, cubic, err := layoutanimation.GenerateWoJsonMoveParams(appName, destName, curveId)
		if err != nil {
			params[i] = moveParams{appName: appName, prepErr: err.Error()}
			continue
		}
		params[i] = moveParams{
			appName:    appName,
			src:        *src,
			dst:        *dst,
			cubic:      *cubic,
			durationMs: durationMs,
		}
	}

	for _, p := range params {
		if p.prepErr != "" {
			continue
		}
		if rerr := s.reorderBeforeMove(p.src.VID, p.appName, p.dst); rerr != nil {
			WLog.Printf("[MoveWindows] reorderBeforeMove %s: %v", p.appName, rerr)
		}
	}

	type animResult struct {
		appName string
		err     error
	}
	results := make([]animResult, len(params))
	var wg sync.WaitGroup
	readyCh := make(chan struct{})

	for i, p := range params {
		if p.prepErr != "" {
			results[i] = animResult{appName: p.appName, err: fmt.Errorf("%s", p.prepErr)}
			continue
		}
		wg.Add(1)
		go func(idx int, mp moveParams) {
			defer wg.Done()
			<-readyCh
			layoutanimation.StartMove(mp.src, mp.dst, mp.cubic, mp.durationMs, nil)
			results[idx] = animResult{appName: mp.appName}
		}(i, p)
	}
	close(readyCh)
	wg.Wait()

	appResults := make([]*uhmi.AppResult, len(results))
	overallOk := true
	for i, r := range results {
		if r.err != nil {
			appResults[i] = &uhmi.AppResult{AppName: r.appName, Status: "error", Info: r.err.Error()}
			overallOk = false
		} else {
			appResults[i] = &uhmi.AppResult{AppName: r.appName, Status: "success"}
		}
	}
	status := "success"
	if !overallOk {
		status = "partial"
	}
	return &uhmi.MoveWindowsResponse{Status: status, Results: appResults}, nil
}

// WidenApp expands an app window across two adjacent virtual displays
// without requiring any JSON configuration files.  The two displays are
// identified by disp_name1 and disp_name2; they must be adjacent (share an
// edge with length > 0).  The bounding box that covers both displays is
// computed and the app's current layer is animated/snapped to that region.
// duration_ms defaults to 0 (immediate snap); curve_id defaults to 3 (linear).
func (s *Server) WidenApp(ctx context.Context, req *uhmi.WidenAppRequest) (*uhmi.WidenAppResponse, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	dispName1 := req.GetDispName1()
	dispName2 := req.GetDispName2()
	durationMs := req.GetDurationMs()
	curveId := int(req.GetCurveId())

	widenedArea, err := layoutparams.GetWidenedRegion(dispName1, dispName2)
	if err != nil {
		return &uhmi.WidenAppResponse{Status: "Failed to WidenApp", Info: err.Error()}, err
	}

	vid, err := layoutcore.ResolveAppLayerVID(appName)
	if err != nil {
		return &uhmi.WidenAppResponse{Status: "Failed to WidenApp", Info: err.Error()}, err
	}
	srcVlayer, err := layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return &uhmi.WidenAppResponse{Status: "Failed to WidenApp", Info: err.Error()}, err
	}

	dstVlayer := *srcVlayer.Dup()
	dstVlayer.VdstX = widenedArea.VirtualX
	dstVlayer.VdstY = widenedArea.VirtualY
	dstVlayer.VdstW = widenedArea.VirtualW
	dstVlayer.VdstH = widenedArea.VirtualH
	dstVlayer.VirtualW = widenedArea.VirtualW
	dstVlayer.VirtualH = widenedArea.VirtualH
	dstVlayer.VsrcX = 0
	dstVlayer.VsrcY = 0
	dstVlayer.VsrcW = widenedArea.VirtualW
	dstVlayer.VsrcH = widenedArea.VirtualH
	for i := range dstVlayer.Vsurfaces {
		dstVlayer.Vsurfaces[i].VdstX = 0
		dstVlayer.Vsurfaces[i].VdstY = 0
		dstVlayer.Vsurfaces[i].VdstW = widenedArea.VirtualW
		dstVlayer.Vsurfaces[i].VdstH = widenedArea.VirtualH
	}

	cubicParam := layoutanimation.CubicParamFromCurveID(curveId)
	if rerr := s.reorderBeforeMove(srcVlayer.VID, appName, dstVlayer); rerr != nil {
		WLog.Printf("[WidenApp] reorderBeforeMove: %v", rerr)
	}
	layoutanimation.StartMove(srcVlayer, dstVlayer, cubicParam, durationMs, nil)

	// modify_vlayer carries no surfaces, so stretching the surfaces over the
	// widened canvas needs its own command.
	if err := sendModifyVsurfaces(dstVlayer); err != nil {
		return &uhmi.WidenAppResponse{Status: "Failed to WidenApp", Info: err.Error()}, err
	}
	return &uhmi.WidenAppResponse{Status: "App widened successfully"}, nil
}

func sendModifyVsurfaces(vlayer layoutcore.VirtualLayer) error {
	for _, vsurf := range vlayer.Vsurfaces {
		cmd, err := layoutcommgen.GenerateCommModifyVsurface(layoutcore.VirtualLayer{
			VID:       vlayer.VID,
			Vsurfaces: []layoutcore.VirtualSurface{vsurf},
		})
		if err != nil {
			return err
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(cmd); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) SetWindowVisibility(ctx context.Context, req *uhmi.SetWindowVisibilityRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	areaName := req.GetAreaName()
	visibility := int(req.GetVisibility())
	vid, err := layoutparams.GetVIDFromDrawAreas(appName, areaName)
	if err != nil {
		return &uhmi.Response{Status: "Failed to LayoutSetWindowVisibility"}, err
	}

	var vlayer layoutcore.VirtualLayer
	vlayer, err = layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return &uhmi.Response{Status: "Failed to LayoutSetWindowVisibility"}, err
	}
	vlayer.Visibility = &visibility

	var layoutComm string
	layoutComm, err = layoutcommgen.GenerateCommModifyVlayer(vlayer)
	if err != nil {
		return &uhmi.Response{Status: "Failed to LayoutSetWindowVisibility"}, err
	}

	err = layoutmulticonn.MulCon.SendLayoutCommand(layoutComm)
	if err != nil {
		return &uhmi.Response{Status: "Failed to LayoutSetWindowVisibility"}, err
	}
	return &uhmi.Response{Status: "Window visibility set successfully"}, nil
}

// ActivateWindow brings all layers of appName to the front of their current
// priority group without changing geometry. If priorityConfig is
// nil the layers are moved to the absolute top of the z-order (global append).
func (s *Server) ActivateWindow(ctx context.Context, req *uhmi.ActivateWindowRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	ILog.Printf("[ActivateWindow] appName=%s", appName)

	layers, found := s.GetCachedAppLayout(appName)
	if !found || len(layers) == 0 {
		return &uhmi.Response{Status: "error", Info: "app not found in layout cache: " + appName}, nil
	}

	if s.priorityEnabled() {
		if err := s.setVisibilityForLayers(layers, 1); err != nil {
			return &uhmi.Response{Status: "error", Info: err.Error()}, nil
		}
		for _, layer := range layers {
			key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
			s.setWindowOrderConstraint(key, windowOrderConstraint{Order: layoutcore.InsertAppend})
		}
		if err := s.publishDesiredOrder(); err != nil {
			return &uhmi.Response{Status: "error", Info: err.Error()}, nil
		}
		s.logLayerHierarchy()
		return &uhmi.Response{Status: "success", Info: "window activated: " + appName}, nil
	}

	ownVIDs := make(map[int]bool, len(layers))
	for _, l := range layers {
		ownVIDs[l.VID] = true
	}
	allLayers := s.getAllCachedLayersSorted()
	otherLayers := make([]layoutcore.VirtualLayer, 0, len(allLayers))
	for _, l := range allLayers {
		if !ownVIDs[l.VID] {
			otherLayers = append(otherLayers, l)
		}
	}

	for _, layer := range layers {
		vlayer, err := layoutvscreen.VScreen.GetVlayerParams(layer.VID)
		if err != nil {
			WLog.Printf("[ActivateWindow] GetVlayerParams VID=%d: %v", layer.VID, err)
			continue
		}

		if vlayer.Visibility == nil || *vlayer.Visibility == 0 {
			one := 1
			vlayer.Visibility = &one
		}

		order, refVid := layoutcore.InsertAppend, -1

		layoutComm, err := layoutcommgen.GenerateCommAddVlayer(vlayer, order, refVid)
		if err != nil {
			return &uhmi.Response{Status: "error", Info: fmt.Sprintf("GenerateCommAddVlayer VID=%d: %v", layer.VID, err)}, nil
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return &uhmi.Response{Status: "error", Info: fmt.Sprintf("SendLayoutCommand VID=%d: %v", layer.VID, err)}, nil
		}
	}

	s.logLayerHierarchy()
	return &uhmi.Response{Status: "success", Info: "window activated: " + appName}, nil
}

// DeactivateWindow hides all layers of appName (visibility=0) and moves them
// to the back of their current priority group. If priorityConfig is nil the
// layers are moved to the absolute bottom of the z-order (global prepend).
func (s *Server) DeactivateWindow(ctx context.Context, req *uhmi.DeactivateWindowRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	ILog.Printf("[DeactivateWindow] appName=%s", appName)

	layers, found := s.GetCachedAppLayout(appName)
	if !found || len(layers) == 0 {
		return &uhmi.Response{Status: "error", Info: "app not found in layout cache: " + appName}, nil
	}

	if s.priorityEnabled() {
		if err := s.setVisibilityForLayers(layers, 0); err != nil {
			return &uhmi.Response{Status: "error", Info: err.Error()}, nil
		}
		for _, layer := range layers {
			key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
			s.setWindowOrderConstraint(key, windowOrderConstraint{Order: layoutcore.InsertPrepend})
		}
		if err := s.publishDesiredOrder(); err != nil {
			return &uhmi.Response{Status: "error", Info: err.Error()}, nil
		}
		s.logLayerHierarchy()
		return &uhmi.Response{Status: "success", Info: "window deactivated: " + appName}, nil
	}

	for _, layer := range layers {
		vlayer, err := layoutvscreen.VScreen.GetVlayerParams(layer.VID)
		if err != nil {
			WLog.Printf("[DeactivateWindow] GetVlayerParams VID=%d: %v", layer.VID, err)
			continue
		}

		zero := 0
		vlayer.Visibility = &zero

		order, refVid := layoutcore.InsertPrepend, -1

		layoutComm, err := layoutcommgen.GenerateCommAddVlayer(vlayer, order, refVid)
		if err != nil {
			return &uhmi.Response{Status: "error", Info: fmt.Sprintf("GenerateCommAddVlayer VID=%d: %v", layer.VID, err)}, nil
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return &uhmi.Response{Status: "error", Info: fmt.Sprintf("SendLayoutCommand VID=%d: %v", layer.VID, err)}, nil
		}
	}

	s.logLayerHierarchy()
	return &uhmi.Response{Status: "success", Info: "window deactivated: " + appName}, nil
}
