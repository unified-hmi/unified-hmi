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

package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"unified-hmi/internal/config"
	layoutcore "unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

const (
	CMD_DistribComm       = "launchApp"
	CMD_RunApp            = "runApp"
	CMD_RunAppAsync       = "runAppAsync"
	CMD_RunAppAsyncCb     = "runAppAsyncCb"
	CMD_StopApp           = "stopApp"
	CMD_GetAppStatus      = "getAppStatus"
	CMD_GetAppComm        = "getAppCmd"
	CMD_ListAvailableApps = "listAvailableApps"
)

type CommTaskContext struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	AppName string
	// Finished is closed when the task's teardown fully completes
	// (all dispatch/receiver cleanup done and the task is removed from the map).
	Finished chan struct{}
	teardown sync.WaitGroup
}

func NewCommTaskCtx(appName string) *CommTaskContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &CommTaskContext{
		Ctx:      ctx,
		Cancel:   cancel,
		AppName:  appName,
		Finished: make(chan struct{}),
	}
}

func (c *CommTaskContext) BeginTeardown() { c.teardown.Add(1) }

func (c *CommTaskContext) EndTeardown() { c.teardown.Done() }

func (c *CommTaskContext) WaitTeardown() { c.teardown.Wait() }

type MultiFlag []string

func BuildRvgpuCmdJSON(entry *layoutcore.AppListEntry, vscrnDef *config.VScrnDef) (string, error) {
	// Fails when the app is not listed in app-list-def.json.
	if _, _, err := layoutcore.ComputeAppVIDs(entry.AppName); err != nil {
		return "", err
	}
	receivers := make([]map[string]interface{}, 0, len(vscrnDef.Def2D.VirtualDisplays))
	for _, display := range vscrnDef.Def2D.VirtualDisplays {
		receivers = append(receivers, map[string]interface{}{
			"display_area": display.DispName,
		})
	}
	cmd := map[string]interface{}{
		"format_v1": map[string]interface{}{
			"appli_name": entry.AppName, "command_type": "remote_virtio_gpu",
			"sender": map[string]interface{}{
				"launcher": entry.Sender,
				"frontend_params": map[string]interface{}{
					"scanout_x": 0, "scanout_y": 0,
					"scanout_w": entry.AppSize.W, "scanout_h": entry.AppSize.H,
				},
				"appli_env": entry.CommandEnv, "appli": entry.AppCommand,
			},
			"receivers": receivers,
		},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *MultiFlag) String() string {
	return strings.Join(*m, ", ")
}
func (m *MultiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// GetAppName extracts format_v1.appli_name from a launch-command JSON payload,
// returning "" if the field is absent or the payload cannot be parsed.
func GetAppName(data []byte) string {
	mJson := make(map[string]interface{})
	err := json.Unmarshal(data, &mJson)
	if err != nil {
		ELog.Printf("Unmarshal json command error: %s \n", err)
		return ""
	}

	formatV1, ok := mJson["format_v1"].(map[string]interface{})
	if !ok {
		return ""
	}
	appName, _ := formatV1["appli_name"].(string)
	return appName
}
