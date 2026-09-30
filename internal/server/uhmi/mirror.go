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

package uhmiserver

import (
	"context"
	"fmt"

	"unified-hmi/internal/config"
	layoutcore "unified-hmi/internal/layout/core"
	layoutparams "unified-hmi/internal/layout/params"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func (s *UhmiServer) cleanupMirrorsOf(ctx context.Context, originalAppName string) {
	mirrorNames := s.UnregisterMirrorsFor(originalAppName)
	for _, m := range mirrorNames {
		if _, err := s.DeleteApplicationLayout(ctx, &uhmi.DeleteApplicationLayoutRequest{AppName: m}); err != nil {
			ELog.Printf("[cleanupMirrorsOf] failed to remove mirror %s of %s: %v", m, originalAppName, err)
			continue
		}
		ILog.Printf("[cleanupMirrorsOf] removed mirror %s of %s", m, originalAppName)
	}
}

func (s *UhmiServer) unmirrorAppByName(ctx context.Context, mirrorName string) (*uhmi.Response, error) {
	if !s.IsMirror(mirrorName) {
		return &uhmi.Response{Status: "error", Info: "not a mirror: " + mirrorName}, nil
	}
	resp, err := s.DeleteApplicationLayout(ctx, &uhmi.DeleteApplicationLayoutRequest{
		AppName: mirrorName,
	})
	if err != nil {
		return &uhmi.Response{Status: "error", Info: "DeleteApplicationLayout failed: " + err.Error()}, nil
	}
	s.UnregisterMirror(mirrorName)
	ILog.Printf("[unmirrorAppByName] mirror removed: %s (layout status: %s)", mirrorName, resp.GetStatus())
	return &uhmi.Response{Status: "success", Info: "mirror removed: " + mirrorName}, nil
}

func (s *UhmiServer) createAppMirror(ctx context.Context, req *uhmi.CreateAppMirrorRequest) (*uhmi.CreateAppMirrorResponse, error) {
	appName := req.GetAppName()
	destName := req.GetDestName()
	ILog.Printf("[CreateAppMirror] appName=%s destName=%s", appName, destName)

	statusResp, _ := s.GetAppStatus(ctx, &uhmi.AppControlRequest{AppName: appName})
	if statusResp.GetInfo() != config.STAT_AppRunning {
		return &uhmi.CreateAppMirrorResponse{Status: "error", Info: "app is not running: " + appName}, nil
	}

	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return &uhmi.CreateAppMirrorResponse{Status: "error", Info: "ReadVScrnDef failed: " + err.Error()}, nil
	}

	mirrorName, mirrorN := s.AllocMirrorName(appName)
	areaName := fmt.Sprintf("mirror%d", mirrorN-1)

	tree, err := buildMirrorLayoutTree(mirrorName, mirrorN, areaName, appName, destName, vscrnDef, s)
	if err != nil {
		return &uhmi.CreateAppMirrorResponse{Status: "error", Info: err.Error()}, nil
	}

	resp, err := s.ApplyApplicationLayout(mirrorName, tree)
	if err != nil {
		return &uhmi.CreateAppMirrorResponse{Status: "error", Info: "ApplyApplicationLayout failed: " + err.Error()}, nil
	}

	s.RegisterMirror(mirrorName, appName, areaName, tree.Vlayers[0].VID)
	ILog.Printf("[CreateAppMirror] mirror created: %s (area=%s layout status: %s)", mirrorName, areaName, resp.GetStatus())
	return &uhmi.CreateAppMirrorResponse{Status: "success", MirrorName: mirrorName}, nil
}

func (s *UhmiServer) removeAppMirror(ctx context.Context, req *uhmi.RemoveAppMirrorRequest) (*uhmi.RemoveAppMirrorResponse, error) {
	mirrorName := req.GetMirrorName()
	ILog.Printf("[RemoveAppMirror] mirrorName=%s", mirrorName)

	if !s.IsMirror(mirrorName) {
		return &uhmi.RemoveAppMirrorResponse{Status: "error", Info: "not a mirror: " + mirrorName}, nil
	}

	resp, err := s.unmirrorAppByName(ctx, mirrorName)
	if err != nil || resp.GetStatus() != "success" {
		info := resp.GetInfo()
		if err != nil {
			info = err.Error()
		}
		return &uhmi.RemoveAppMirrorResponse{Status: "error", Info: info}, nil
	}
	return &uhmi.RemoveAppMirrorResponse{Status: "success", Info: resp.GetInfo()}, nil
}

func buildMirrorLayoutTree(mirrorName string, mirrorN int, areaName, originalAppName, destName string, vscrnDef *config.VScrnDef, layout *UhmiServer) (*layoutcore.LayoutTree, error) {
	dst, err := layoutparams.GetMoveDestRegion(destName)
	if err != nil {
		return nil, fmt.Errorf("buildMirrorLayoutTree: %w", err)
	}
	vx, vy, vw, vh := dst.VirtualX, dst.VirtualY, dst.VirtualW, dst.VirtualH

	origSurfaceVID := 0
	var appPixelW, appPixelH float64
	if origLayers, found := layout.GetCachedAppLayout(originalAppName); found && len(origLayers) > 0 {
		for _, layer := range origLayers {
			if len(layer.Vsurfaces) > 0 {
				origSurfaceVID = layer.Vsurfaces[0].VID
				appPixelW = layer.Vsurfaces[0].PixelW
				appPixelH = layer.Vsurfaces[0].PixelH
				break
			}
		}
	}
	if origSurfaceVID == 0 {
		_, sid, err := layoutcore.ComputeAppVIDs(originalAppName)
		if err != nil {
			return nil, fmt.Errorf("buildMirrorLayoutTree: %w", err)
		}
		origSurfaceVID = sid
	}

	if appPixelW <= 0 || appPixelH <= 0 {
		entry, err := layoutcore.ReadAppListEntry(originalAppName)
		if err == nil && entry != nil {
			appPixelW = float64(entry.AppSize.W)
			appPixelH = float64(entry.AppSize.H)
		}
	}
	if appPixelW <= 0 {
		appPixelW = vw
	}
	if appPixelH <= 0 {
		appPixelH = vh
	}

	mirrorLayerVID, err := layoutcore.ComputeMirrorLayerVID(originalAppName, mirrorN)
	if err != nil {
		return nil, fmt.Errorf("buildMirrorLayoutTree: %w", err)
	}

	vis := 1
	tree := &layoutcore.LayoutTree{
		Vlayers: []layoutcore.VirtualLayer{
			{
				AppName:    originalAppName,
				AreaName:   areaName,
				VID:        mirrorLayerVID,
				Coord:      layoutcore.COORD_GLOBAL,
				VirtualW:   vw,
				VirtualH:   vh,
				VsrcX:      0,
				VsrcY:      0,
				VsrcW:      vw,
				VsrcH:      vh,
				VdstX:      vx,
				VdstY:      vy,
				VdstW:      vw,
				VdstH:      vh,
				Visibility: &vis,
				Vsurfaces: []layoutcore.VirtualSurface{
					{
						AppName:    originalAppName,
						ParentVID:  mirrorLayerVID,
						VID:        origSurfaceVID,
						PixelW:     appPixelW,
						PixelH:     appPixelH,
						PsrcX:      0,
						PsrcY:      0,
						PsrcW:      appPixelW,
						PsrcH:      appPixelH,
						VdstX:      0,
						VdstY:      0,
						VdstW:      vw,
						VdstH:      vh,
						Visibility: &vis,
					},
				},
			},
		},
	}
	return tree, nil
}
