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
	"encoding/json"
	"strconv"
	"strings"

	layoutcore "unified-hmi/internal/layout/core"
	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
)

const (
	appSourceJSON    = "app.json"
	appSourceListDef = "app-list-def.json"
)

// AvailableApp identifies an application and the configuration that defines it.
type AvailableApp struct {
	Name   string
	Source string
}

func getAppCmdFromEachNode(appName string) []byte {
	fNodes, err := gVScrnDef.GetFrameworkNode()
	if err != nil {
		return nil
	}
	DLog.Println("GetFrameworkNode ", fNodes)

	cNodes := lifecycle.GetConnectableNode(fNodes)
	rcvDataCh := make(chan []byte, len(cNodes))
	for _, fnip := range cNodes {
		targetAddr := fnip.Ip + ":" + strconv.Itoa(fnip.Port)
		go lifecycle.GetAppInfoFromNode(targetAddr, appName, lifecycle.CMD_GetAppComm, rcvDataCh)
	}

	var appCommand []byte
	for i := 0; i < len(cNodes); i++ {
		select {
		case rcvData := <-rcvDataCh:
			if rcvData != nil {
				appCommand = rcvData
			}
		}
	}

	return appCommand
}

func getAppCmd(appName string) []byte {
	var command []byte
	command = getAppCmdFromEachNode(appName)
	if command == nil {
		ELog.Printf("getAppCmdFromEachNode error")
		return nil
	}

	return command
}

func getAppNamesFromEachNode() ([]string, error) {
	fNodes, err := gVScrnDef.GetFrameworkNode()
	if err != nil {
		return nil, err
	}
	DLog.Println("GetFrameworkNode ", fNodes)

	cNodes := lifecycle.GetConnectableNode(fNodes)
	data, _ := json.Marshal(cNodes)
	rcvDataCh := make(chan []byte, len(cNodes))
	for _, fnip := range cNodes {
		targetAddr := fnip.Ip + ":" + strconv.Itoa(fnip.Port)
		go lifecycle.GetAppInfoFromNode(targetAddr, string(data), lifecycle.CMD_ListAvailableApps, rcvDataCh)
	}

	var appNames []string
	for i := 0; i < len(cNodes); i++ {
		select {
		case rcvData := <-rcvDataCh:
			if rcvData == nil {
				continue
			}
			appNames = append(appNames, strings.Split(string(rcvData), ",")...)
		}
	}

	return uniqueAppNames(appNames), nil
}

func uniqueAppNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	apps := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		apps = append(apps, name)
	}
	return apps
}

func mergeAvailableApps(appJSONNames []string, entries []layoutcore.AppListEntry) []AvailableApp {
	available := make([]AvailableApp, 0, len(appJSONNames)+len(entries))
	seen := make(map[string]struct{}, len(appJSONNames)+len(entries))
	for _, name := range uniqueAppNames(appJSONNames) {
		seen[name] = struct{}{}
		available = append(available, AvailableApp{Name: name, Source: appSourceJSON})
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.AppName)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		available = append(available, AvailableApp{Name: name, Source: appSourceListDef})
	}
	return available
}

func formatAvailableApps(apps []AvailableApp) string {
	formatted := make([]string, 0, len(apps))
	for _, app := range apps {
		formatted = append(formatted, app.Name+"("+app.Source+")")
	}
	return strings.Join(formatted, ",")
}
