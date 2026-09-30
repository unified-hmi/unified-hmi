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
	"time"

	"unified-hmi/internal/config"
	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcore "unified-hmi/internal/layout/core"
	layoutparams "unified-hmi/internal/layout/params"
	"unified-hmi/internal/lifecycle"
	layoutserver "unified-hmi/internal/server/layout"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func (s *UhmiServer) launchApp(ctx context.Context, req *uhmi.LaunchAppRequest) (*uhmi.LaunchAppResponse, error) {
	syncRun := !req.GetAsync()
	status, info, err := s.launchAppCore(ctx, req.GetAppName(), req.GetDestName(), syncRun)
	if err != nil {
		return &uhmi.LaunchAppResponse{Status: "error", Info: err.Error()}, nil
	}
	return &uhmi.LaunchAppResponse{Status: status, Info: info}, nil
}

type launchLayoutMode int

const (
	layoutModeDest launchLayoutMode = iota
	layoutModeJson
)

func (s *UhmiServer) launchAppCore(ctx context.Context, appName, destName string, syncRun bool) (status, info string, err error) {
	ILog.Printf("[LaunchApp] appName=%s destName=%s syncRun=%v", appName, destName, syncRun)

	vscrnDef, verr := config.ReadVScrnDef()
	if verr != nil {
		return "error", "ReadVScrnDef failed: " + verr.Error(), nil
	}

	cmdJSON, mode, tree, perr := s.prepareLaunchApp(ctx, appName, destName, vscrnDef)
	if perr != nil {
		return "error", perr.Error(), nil
	}

	var launchDone chan error
	if cmdJSON == "" {
		ILog.Printf("[LaunchApp] app already running, skipping launch: app=%s", appName)
	} else if syncRun {
		launchDone = make(chan error, 1)
		go func() {
			_, runErr := s.RunAppCommand(ctx, &uhmi.AppCommandRequest{AppJson: cmdJSON})
			launchDone <- runErr
		}()
	} else {
		go func() {
			_, runErr := s.RunAppCommand(context.Background(), &uhmi.AppCommandRequest{AppJson: cmdJSON})
			if runErr != nil {
				ELog.Printf("[LaunchApp] RunAppCommand failed: app=%s err=%v", appName, runErr)
			}
		}()
	}

	time.Sleep(2 * time.Second)

	DLog.Printf("[LaunchApp] cmdJSON=%s", cmdJSON)

	switch mode {
	case layoutModeDest:
		layoutcore.PrintLayoutTree(tree)
		resp, aerr := s.ApplyApplicationLayout(appName, tree)
		if aerr != nil {
			status, info = "error", "ApplyApplicationLayout failed: "+aerr.Error()
		} else {
			status, info = "success", resp.GetStatus()
		}
	case layoutModeJson:
		DLog.Printf("[LaunchApp] applying initial_layout.json for app=%s", appName)
		resp, aerr := s.SetApplicationLayout(ctx, &uhmi.SetApplicationLayoutRequest{AppName: appName})
		if aerr != nil {
			status, info = "error", "SetApplicationLayout failed: "+aerr.Error()
		} else {
			status, info = "success", resp.GetStatus()
		}
	default:
		status, info = "error", "unknown layout mode"
	}

	if launchDone != nil {
		if runErr := <-launchDone; runErr != nil {
			ELog.Printf("[LaunchApp] RunAppCommand failed: app=%s err=%v", appName, runErr)
		}
	}

	return status, info, nil
}

func (s *UhmiServer) prepareLaunchApp(ctx context.Context, appName, destName string, vscrnDef *config.VScrnDef) (cmdJSON string, mode launchLayoutMode, tree *layoutcore.LayoutTree, err error) {
	if destName != "" {
		entry, err := layoutcore.ReadAppListEntry(appName)
		if err != nil {
			return "", layoutModeDest, nil, fmt.Errorf("readAppListEntry failed: %w", err)
		}
		if entry == nil {
			return "", layoutModeDest, nil, fmt.Errorf("app not found in app-list-def.json: %s", appName)
		}
		tree, err = layoutparams.BuildLayoutTreeWoJson(appName, destName, entry, vscrnDef)
		if err != nil {
			return "", layoutModeDest, nil, fmt.Errorf("buildLayoutTreeWoJson failed: %w", err)
		}
		mode = layoutModeDest
	} else {
		if _, lerr := layoutclusterapp.ReadLayoutTreeFromCfgForApp(appName); lerr != nil {
			return "", layoutModeJson, nil, fmt.Errorf("destName is required: no initial_layout.json found for app %q (%v)", appName, lerr)
		}
		mode = layoutModeJson
	}

	statusResp, _ := s.GetAppStatus(ctx, &uhmi.AppControlRequest{AppName: appName})
	if statusResp.GetInfo() == config.STAT_AppRunning {
		return "", mode, tree, nil
	}

	appCmdResp, _ := s.GetAppCommand(ctx, &uhmi.AppControlRequest{AppName: appName})
	if appCmdResp.GetStatus() == config.STAT_ExecFin && appCmdResp.GetInfo() != "" {
		ILog.Printf("[LaunchApp] launching via app.json: app=%s", appName)
		return appCmdResp.GetInfo(), mode, tree, nil
	}

	ILog.Printf("[LaunchApp] app.json not found, generating command: app=%s", appName)
	entry, err := layoutcore.ReadAppListEntry(appName)
	if err != nil {
		return "", mode, nil, fmt.Errorf("readAppListEntry failed: %w", err)
	}
	if entry == nil {
		return "", mode, nil, fmt.Errorf("app not found in app-list-def.json: %s", appName)
	}
	cmdJSON, err = lifecycle.BuildRvgpuCmdJSON(entry, vscrnDef)
	if err != nil {
		return "", mode, nil, fmt.Errorf("buildRvgpuCmdJSON failed: %w", err)
	}
	return cmdJSON, mode, tree, nil
}

func (s *UhmiServer) launchApps(ctx context.Context, req *uhmi.LaunchAppsRequest) (*uhmi.LaunchAppsResponse, error) {
	apps := req.GetApps()
	if len(apps) == 0 {
		return &uhmi.LaunchAppsResponse{Status: "error", Info: "no apps specified"}, nil
	}
	ILog.Printf("[LaunchApps] count=%d", len(apps))

	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return &uhmi.LaunchAppsResponse{Status: "error", Info: "ReadVScrnDef failed: " + err.Error()}, nil
	}

	type preparedApp struct {
		appName string
		cmdJSON string
		tree    *layoutcore.LayoutTree
		prepErr string
	}
	prepared := make([]preparedApp, len(apps))
	for i, a := range apps {
		appName := a.GetAppName()
		destName := a.GetDestName()
		cmdJSON, tree, err := s.prepareWoJsonApp(ctx, appName, destName, vscrnDef)
		if err != nil {
			prepared[i] = preparedApp{appName: appName, prepErr: err.Error()}
			continue
		}
		prepared[i] = preparedApp{appName: appName, cmdJSON: cmdJSON, tree: tree}
	}

	readyCh := make(chan struct{})
	for _, p := range prepared {
		if p.prepErr != "" || p.cmdJSON == "" {
			continue
		}
		go func(appName, cmdJSON string) {
			<-readyCh
			_, runErr := s.RunAppCommand(context.Background(), &uhmi.AppCommandRequest{AppJson: cmdJSON})
			if runErr != nil {
				ELog.Printf("[LaunchApps] RunAppCommand failed: app=%s err=%v", appName, runErr)
			}
		}(p.appName, p.cmdJSON)
	}
	close(readyCh)

	time.Sleep(4 * time.Second)

	type layoutResult struct {
		appName string
		err     error
	}
	layoutResults := make([]layoutResult, len(prepared))

	entries := make([]layoutserver.ApplyMultiAppEntry, 0, len(prepared))
	for _, p := range prepared {
		if p.prepErr != "" {
			continue
		}
		entries = append(entries, layoutserver.ApplyMultiAppEntry{AppName: p.appName, Tree: p.tree})
	}

	var batchErr error
	if len(entries) > 0 {
		_, batchErr = s.ApplyMultipleApplicationLayouts(entries)
	}

	for i, p := range prepared {
		if p.prepErr != "" {
			layoutResults[i] = layoutResult{appName: p.appName, err: fmt.Errorf("%s", p.prepErr)}
		} else {
			layoutResults[i] = layoutResult{appName: p.appName, err: batchErr}
		}
	}

	results := make([]*uhmi.AppResult, len(prepared))
	overallOk := true
	for i, lr := range layoutResults {
		if lr.err != nil {
			results[i] = &uhmi.AppResult{AppName: lr.appName, Status: "error", Info: lr.err.Error()}
			overallOk = false
		} else {
			results[i] = &uhmi.AppResult{AppName: lr.appName, Status: "success"}
		}
	}
	status := "success"
	if !overallOk {
		status = "partial"
	}
	return &uhmi.LaunchAppsResponse{Status: status, Results: results}, nil
}

func (s *UhmiServer) prepareWoJsonApp(ctx context.Context, appName, destName string, vscrnDef *config.VScrnDef) (cmdJSON string, tree *layoutcore.LayoutTree, err error) {
	entry, err := layoutcore.ReadAppListEntry(appName)
	if err != nil {
		return "", nil, fmt.Errorf("readAppListEntry failed: %w", err)
	}
	if entry == nil {
		return "", nil, fmt.Errorf("app not found in app-list-def.json: %s", appName)
	}
	tree, err = layoutparams.BuildLayoutTreeWoJson(appName, destName, entry, vscrnDef)
	if err != nil {
		return "", nil, fmt.Errorf("buildLayoutTreeWoJson failed: %w", err)
	}
	statusResp, _ := s.GetAppStatus(ctx, &uhmi.AppControlRequest{AppName: appName})
	if statusResp.GetInfo() != config.STAT_AppRunning {
		cmdJSON, err = lifecycle.BuildRvgpuCmdJSON(entry, vscrnDef)
		if err != nil {
			return "", nil, fmt.Errorf("buildRvgpuCmdJSON failed: %w", err)
		}
	}
	return cmdJSON, tree, nil
}
