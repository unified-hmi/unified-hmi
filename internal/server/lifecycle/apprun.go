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
	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func (s *Server) RunAppCommand(grpcCtx context.Context, req *uhmi.AppCommandRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecErr

	appJson := req.GetAppJson()
	appName := lifecycle.GetAppName([]byte(appJson))
	if appName == "" {
		ELog.Printf("json command error")
		return &uhmi.Response{Status: responseStatus}, nil
	}

	commTaskCtx, err := updateCommTaskCtxMap(appName)
	if err != nil {
		responseStatus = config.STAT_ExecBusy
		return &uhmi.Response{Status: responseStatus}, nil
	}

	command := []byte(appJson)
	if command == nil {
		ELog.Printf("No json command error")
		deleteCommTask(appName)
		return &uhmi.Response{Status: responseStatus}, nil
	}

	go func() {
		defer deleteCommTask(appName)
		dispatchAppWithReceivers(command, commTaskCtx)
	}()

	select {
	case <-commTaskCtx.Ctx.Done():
		responseStatus = config.STAT_ExecFin

	case <-grpcCtx.Done():
		ELog.Printf("(task=%s) [CANCEL_REASON] RunAppCommand: gRPC client disconnected or request cancelled (reason: %v)", appName, grpcCtx.Err())
		responseStatus = config.STAT_ExecFin
		commTaskCtx.Cancel()
	}

	return &uhmi.Response{Status: responseStatus}, nil
}

func (s *Server) RunApp(grpcCtx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecErr

	appName := req.GetAppName()
	commTaskCtx, err := updateCommTaskCtxMap(appName)
	if err != nil {
		responseStatus = config.STAT_ExecBusy
		return &uhmi.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", appName)
		deleteCommTask(appName)
		return &uhmi.Response{Status: responseStatus}, nil
	}

	go func() {
		defer deleteCommTask(appName)
		dispatchAppWithReceivers(command, commTaskCtx)
	}()

	select {
	case <-commTaskCtx.Ctx.Done():
		responseStatus = config.STAT_ExecFin

	case <-grpcCtx.Done():
		ELog.Printf("(task=%s) [CANCEL_REASON] RunApp: gRPC client disconnected or request cancelled (reason: %v)", appName, grpcCtx.Err())
		responseStatus = config.STAT_ExecFin
		commTaskCtx.Cancel()
	}

	return &uhmi.Response{Status: responseStatus}, nil
}

func (s *Server) RunAppAsync(grpcCtx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecSuccess

	appName := req.GetAppName()
	commTaskCtx, err := updateCommTaskCtxMap(appName)
	if err != nil {
		responseStatus = config.STAT_ExecBusy
		return &uhmi.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", appName)
		deleteCommTask(appName)
		return &uhmi.Response{Status: responseStatus}, nil
	}

	go func() {
		defer deleteCommTask(appName)
		dispatchAppWithReceivers(command, commTaskCtx)
	}()

	return &uhmi.Response{Status: responseStatus}, nil
}

func (s *Server) StopAppBase(grpcCtx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	responseStatus := config.STAT_ExecErr

	if err := cancelCommTaskCtx(grpcCtx, req.GetAppName()); err == nil {
		responseStatus = config.STAT_ExecFin
	} else {
		ELog.Printf("(task=%s) StopApp: teardown did not complete: %v", req.GetAppName(), err)
	}

	return &uhmi.Response{Status: responseStatus}, nil
}

// StopAllApps stops every running app, blocking until each teardown fully
// completes (or ctx is done).
func (s *Server) StopAllApps(ctx context.Context) error {
	if err := cancelAllCommTaskCtx(ctx); err != nil {
		ELog.Printf("StopAllApps: teardown did not complete for all tasks: %v", err)
		return err
	}

	return nil
}
