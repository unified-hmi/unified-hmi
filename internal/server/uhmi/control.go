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
	"os"
	"strings"
	"time"

	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func (s *UhmiServer) stopApp(ctx context.Context, req *uhmi.AppControlRequest) (*uhmi.Response, error) {
	if s.IsMirror(req.GetAppName()) {
		return s.unmirrorAppByName(ctx, req.GetAppName())
	}
	s.cleanupMirrorsOf(ctx, req.GetAppName())
	return s.lifecycle.StopAppBase(ctx, req)
}

func (s *UhmiServer) startAll(ctx context.Context, in *uhmi.Empty) (*uhmi.Response, error) {
	fmt.Fprintln(os.Stderr, "[uhmi-master-node] StartAll called ----")

	apps, err := s.lifecycle.AutoStartAppNames()
	if err != nil || len(apps) == 0 {
		return &uhmi.Response{Status: "error", Info: "no executable apps found"}, nil
	}

	for _, appName := range apps {
		fmt.Fprintf(os.Stderr, "[uhmi-master-node] processing app: %s ----\n", appName)

		cmdResp, err := s.GetAppCommand(ctx, &uhmi.AppControlRequest{AppName: appName})
		if err != nil || cmdResp.GetStatus() != "Finish" {
			return &uhmi.Response{Status: "error", Info: "failed to get app command: " + appName}, nil
		}

		go func(json string, name string) {
			_, err := s.RunAppCommand(context.Background(), &uhmi.AppCommandRequest{AppJson: json})
			if err != nil {
				ELog.Printf("RunAppCommand failed: app=%s err=%v", name, err)
			}
		}(cmdResp.GetInfo(), appName)
	}

	time.Sleep(2 * time.Second)

	if _, err := s.ReloadConfig(ctx, &uhmi.Empty{}); err != nil {
		return &uhmi.Response{Status: "error", Info: "failed to configure LAYOUT"}, nil
	}

	if _, err := s.ApplySystemLayout(ctx, &uhmi.Empty{}); err != nil {
		return &uhmi.Response{Status: "error", Info: "failed to set system layout"}, nil
	}

	return &uhmi.Response{Status: "success", Info: "uhmi started"}, nil
}

func (s *UhmiServer) stopAll(ctx context.Context, in *uhmi.Empty) (*uhmi.Response, error) {
	fmt.Fprintln(os.Stderr, "[uhmi-master-node] StopAll called ----")

	var cleanupFailures []string
	for _, mirrorName := range s.GetMirrorNames() {
		if _, err := s.layout.DeleteApplicationLayout(ctx, &uhmi.DeleteApplicationLayoutRequest{AppName: mirrorName}); err != nil {
			cleanupFailures = append(cleanupFailures, fmt.Sprintf("%s: %v", mirrorName, err))
			ELog.Printf("[StopAll] failed to remove mirror %s: %v", mirrorName, err)
			continue
		}
		s.UnregisterMirror(mirrorName)
		ILog.Printf("[StopAll] removed mirror %s", mirrorName)
	}
	if len(cleanupFailures) > 0 {
		return &uhmi.Response{Status: "error", Info: "failed to remove mirror layouts: " + strings.Join(cleanupFailures, "; ")}, nil
	}

	if err := s.lifecycle.StopAllApps(ctx); err != nil {
		return &uhmi.Response{Status: "error", Info: "one or more apps did not finish stopping: " + err.Error()}, nil
	}

	s.layout.PurgeLayoutCache()
	return &uhmi.Response{Status: "success", Info: "uhmi stopped"}, nil
}
