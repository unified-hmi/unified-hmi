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

// Package uhmiserver implements UhmiServer, the single gRPC server for the
// unified UHMIService (proto/uhmi.proto); every RPC delegates to the LAYOUT
// domain, the LIFECYCLE domain, or this package's system orchestration.
package uhmiserver

import (
	"context"
	"sync"

	"unified-hmi/internal/config"
	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcore "unified-hmi/internal/layout/core"
	layoutmulticonn "unified-hmi/internal/layout/multiconn"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	"unified-hmi/internal/server/layout"
	"unified-hmi/internal/server/lifecycle"
	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

// UhmiServer implements uhmi.UHMIServiceServer as a single unified server
// that composes the LAYOUT window-management domain, the LIFECYCLE handlers and
// the system state machine. UnsafeUHMIServiceServer opts out of forward
// compatibility so the interface must be satisfied explicitly.
type UhmiServer struct {
	uhmi.UnsafeUHMIServiceServer

	layout    *layoutserver.Server
	lifecycle *lifecycleserver.Server

	layoutMutationMu sync.Mutex
}

var _ uhmi.UHMIServiceServer = (*UhmiServer)(nil)

// NewUhmiServer creates the unified server: it initializes the virtual screen,
// cluster-app configuration and compositor connections (LAYOUT) and stores the
// shared virtual-screen definition (LIFECYCLE).
func NewUhmiServer(vscrnDef *config.VScrnDef) (*UhmiServer, error) {
	var err error
	layoutvscreen.VScreen, err = layoutvscreen.NewVirtualScreen(vscrnDef)
	if err != nil {
		ELog.Printf("Failed to Create VirtualScreen: %s\n", err)
		return nil, err
	}
	layoutclusterapp.ClusterAppConfigure()
	force := config.GetEnvBool("LAYOUT_FORCE", false)
	if err = layoutmulticonn.ConnectionInit(vscrnDef, force); err != nil {
		ELog.Printf("Failed to Init Connection: %s\n", err)
		return nil, err
	}

	srv := &UhmiServer{
		layout:    layoutserver.NewServer(),
		lifecycle: lifecycleserver.NewServer(vscrnDef),
	}
	return srv, nil
}

// ReloadConfig re-reads the cluster-app configuration (LAYOUT) and the
// priority-group policy.
func (s *UhmiServer) ReloadConfig(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	serverutil.LogFunc()
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	layoutclusterapp.ClusterAppConfigure()
	s.layout.ReloadPriorityConfig()
	return &uhmi.Response{Status: "LAYOUT configure successfully"}, nil
}

// ===== System orchestration =====
func (s *UhmiServer) StartAll(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	return s.startAll(ctx, req)
}
func (s *UhmiServer) StopAll(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.stopAll(ctx, req)
}

// ===== Application lifecycle =====
func (s *UhmiServer) ListAvailableApps(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	return s.lifecycle.ListAvailableApps(ctx, req)
}
func (s *UhmiServer) ListRunningApps(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	return s.listRunningApps(ctx, req)
}
func (s *UhmiServer) GetAppCommand(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	return s.lifecycle.GetAppCommand(ctx, req)
}
func (s *UhmiServer) GetAppStatus(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	return s.lifecycle.GetAppStatus(ctx, req)
}
func (s *UhmiServer) RunAppCommand(ctx context.Context, req *uhmi.AppCommandRequest) (*uhmi.Response, error) {
	return s.lifecycle.RunAppCommand(ctx, req)
}
func (s *UhmiServer) RunApp(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	return s.lifecycle.RunApp(ctx, req)
}
func (s *UhmiServer) RunAppAsync(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	return s.lifecycle.RunAppAsync(ctx, req)
}
func (s *UhmiServer) StopApp(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	return s.stopApp(ctx, req)
}

// ===== App launch & placement =====
func (s *UhmiServer) LaunchApp(ctx context.Context, req *uhmi.LaunchAppRequest) (*uhmi.LaunchAppResponse, error) {
	return s.launchApp(ctx, req)
}
func (s *UhmiServer) LaunchApps(ctx context.Context, req *uhmi.LaunchAppsRequest) (*uhmi.LaunchAppsResponse, error) {
	return s.launchApps(ctx, req)
}
func (s *UhmiServer) MoveWindow(ctx context.Context, req *uhmi.MoveWindowRequest) (*uhmi.MoveWindowResponse, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.MoveWindow(ctx, req)
}
func (s *UhmiServer) MoveWindows(ctx context.Context, req *uhmi.MoveWindowsRequest) (*uhmi.MoveWindowsResponse, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.MoveWindows(ctx, req)
}
func (s *UhmiServer) WidenApp(ctx context.Context, req *uhmi.WidenAppRequest) (*uhmi.WidenAppResponse, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.WidenApp(ctx, req)
}
func (s *UhmiServer) ListDestinations(ctx context.Context, req *uhmi.ListDestinationsRequest) (*uhmi.ListDestinationsResponse, error) {
	return s.listDestinations(ctx, req)
}

// ===== App mirroring =====
func (s *UhmiServer) CreateAppMirror(ctx context.Context, req *uhmi.CreateAppMirrorRequest) (*uhmi.CreateAppMirrorResponse, error) {
	return s.createAppMirror(ctx, req)
}
func (s *UhmiServer) RemoveAppMirror(ctx context.Context, req *uhmi.RemoveAppMirrorRequest) (*uhmi.RemoveAppMirrorResponse, error) {
	return s.removeAppMirror(ctx, req)
}

// ===== System layout & animation =====
func (s *UhmiServer) ApplySystemLayout(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.ApplySystemLayout(ctx, req)
}
func (s *UhmiServer) SetApplicationLayout(ctx context.Context, req *uhmi.SetApplicationLayoutRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.SetApplicationLayout(ctx, req)
}
func (s *UhmiServer) ReSyncSystemLayout(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	return s.layout.ReSyncSystemLayout(ctx, req)
}
func (s *UhmiServer) SetLayoutCommand(ctx context.Context, req *uhmi.SetLayoutCommandRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.SetLayoutCommand(ctx, req)
}
func (s *UhmiServer) StartSystemAnimation(ctx context.Context, req *uhmi.StartSystemAnimationRequest) (*uhmi.Response, error) {
	return s.layout.StartSystemAnimation(ctx, req)
}
func (s *UhmiServer) StartSystemAnimationAsync(ctx context.Context, req *uhmi.StartSystemAnimationAsyncRequest) (*uhmi.Response, error) {
	return s.layout.StartSystemAnimationAsync(ctx, req)
}
func (s *UhmiServer) WaitAsyncAnimation(ctx context.Context, req *uhmi.WaitAsyncAnimationRequest) (*uhmi.Response, error) {
	return s.layout.WaitAsyncAnimation(ctx, req)
}
func (s *UhmiServer) WaitAllAsyncAnimations(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	return s.layout.WaitAllAsyncAnimations(ctx, req)
}
func (s *UhmiServer) GetAnimationInfo(ctx context.Context, req *uhmi.GetAnimationInfoRequest) (*uhmi.GetAnimationInfoResponse, error) {
	return s.layout.GetAnimationInfo(ctx, req)
}

// ===== Window activation & priority =====
func (s *UhmiServer) ActivateWindow(ctx context.Context, req *uhmi.ActivateWindowRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.ActivateWindow(ctx, req)
}
func (s *UhmiServer) DeactivateWindow(ctx context.Context, req *uhmi.DeactivateWindowRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.DeactivateWindow(ctx, req)
}
func (s *UhmiServer) SetAppPriorityGroup(ctx context.Context, req *uhmi.SetAppPriorityGroupRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.SetAppPriorityGroup(ctx, req)
}
func (s *UhmiServer) ListPriorityGroups(ctx context.Context, req *uhmi.Empty) (*uhmi.ListPriorityGroupsResponse, error) {
	return s.layout.ListPriorityGroups(ctx, req)
}

// ===== Notifications =====
func (s *UhmiServer) SubscribeNotifications(req *uhmi.Empty, stream uhmi.UHMIService_SubscribeNotificationsServer) error {
	return s.layout.SubscribeNotifications(req, stream)
}

// ApplyApplicationLayout forwards to the LAYOUT domain.
func (s *UhmiServer) ApplyApplicationLayout(appName string, appTree *layoutcore.LayoutTree) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.ApplyApplicationLayout(appName, appTree)
}

// ApplyMultipleApplicationLayouts forwards to the LAYOUT domain.
func (s *UhmiServer) ApplyMultipleApplicationLayouts(entries []layoutserver.ApplyMultiAppEntry) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.ApplyMultipleApplicationLayouts(entries)
}

// DeleteApplicationLayout forwards to the LAYOUT domain.
func (s *UhmiServer) DeleteApplicationLayout(ctx context.Context, req *uhmi.DeleteApplicationLayoutRequest) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	return s.layout.DeleteApplicationLayout(ctx, req)
}

// IsMirror forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) IsMirror(name string) bool {
	return s.layout.IsMirror(name)
}

// AllocMirrorName forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) AllocMirrorName(originalAppName string) (name string, n int) {
	return s.layout.AllocMirrorName(originalAppName)
}

// RegisterMirror forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) RegisterMirror(mirrorName, originalAppName, areaName string, vid int) {
	s.layout.RegisterMirror(mirrorName, originalAppName, areaName, vid)
}

// UnregisterMirror forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) UnregisterMirror(mirrorName string) {
	s.layout.UnregisterMirror(mirrorName)
}

// UnregisterMirrorsFor forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) UnregisterMirrorsFor(originalAppName string) []string {
	return s.layout.UnregisterMirrorsFor(originalAppName)
}

// DrainAllMirrors forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) DrainAllMirrors() []string {
	return s.layout.DrainAllMirrors()
}

// GetMirrorNames forwards to the LAYOUT domain's mirror registry.
func (s *UhmiServer) GetMirrorNames() []string {
	return s.layout.GetMirrorNames()
}

// GetCachedAppLayout forwards to the LAYOUT domain's layout cache.
func (s *UhmiServer) GetCachedAppLayout(appName string) ([]layoutcore.VirtualLayer, bool) {
	return s.layout.GetCachedAppLayout(appName)
}
