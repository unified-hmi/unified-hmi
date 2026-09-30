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

package lifecycleserver

import (
	"context"

	"unified-hmi/internal/config"
	layoutcore "unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

var (
	appNamesFromNodes  = getAppNamesFromEachNode
	readAppListEntries = layoutcore.ReadAppListEntries
)

// AvailableApps returns all known applications with their defining source.
func (s *Server) AvailableApps() ([]AvailableApp, error) {
	appJSONNames, workerErr := appNamesFromNodes()
	entries, entryErr := readAppListEntries()
	if entryErr != nil {
		WLog.Printf("ReadAppListEntries: %v", entryErr)
		if workerErr != nil {
			return nil, workerErr
		}
		return mergeAvailableApps(appJSONNames, nil), nil
	}
	if workerErr != nil {
		WLog.Printf("getAppNamesFromEachNode: %v", workerErr)
	}
	return mergeAvailableApps(appJSONNames, entries), nil
}

// AutoStartAppNames returns worker-validated app.json application names.
func (s *Server) AutoStartAppNames() ([]string, error) {
	return appNamesFromNodes()
}

func (s *Server) ListAvailableApps(grpcCtx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	// An empty app list is a valid answer, so only a lookup error fails.
	apps, err := s.AvailableApps()
	if err != nil {
		ELog.Printf("LifecycleListAvailableApps: %v", err)
		return &uhmi.Response{Status: config.STAT_ExecErr, Info: ""}, nil
	}

	return &uhmi.Response{Status: config.STAT_ExecFin, Info: formatAvailableApps(apps)}, nil
}

func (s *Server) ListRunningAppsBase(grpcCtx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	// Having no app running is a valid answer, not a failure.
	appList := getRunningAppFromCommTaskCtxMap()

	return &uhmi.Response{Status: config.STAT_ExecFin, Info: string(appList)}, nil
}

func (s *Server) GetAppCommand(grpcCtx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecErr
	infoData := ""

	appName := req.GetAppName()
	if appName == "" {
		ELog.Printf("LifecycleGetAppCommand: empty app name")
		return &uhmi.Response{Status: responseStatus, Info: infoData}, nil
	}

	appCommand := getAppCmdFromEachNode(appName)
	if appCommand != nil {
		responseStatus = config.STAT_ExecFin
		infoData = string(appCommand)
	}

	return &uhmi.Response{Status: responseStatus, Info: infoData}, nil
}

func (s *Server) GetAppStatus(grpcCtx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecFin
	responseInfo := config.STAT_AppStop

	appName := req.GetAppName()
	if exists := isExistsCommTaskCtx(appName); exists {
		responseInfo = config.STAT_AppRunning
	}

	return &uhmi.Response{Status: responseStatus, Info: responseInfo}, nil
}
