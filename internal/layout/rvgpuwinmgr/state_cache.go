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

package rvgpuwinmgr

import (
	"encoding/json"
	"sync"

	layoutcore "unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

var (
	latestStateMu sync.RWMutex
	latestState   *layoutcore.NodePixelScreens
)

func setLatestState(sps *layoutcore.NodePixelScreens) {
	if sps == nil {
		return
	}
	latestStateMu.Lock()
	defer latestStateMu.Unlock()
	latestState = sps.Dup()
}

func buildInitialLayoutForDisplay(rId int) (string, error) {
	latestStateMu.RLock()
	sps := latestState
	latestStateMu.RUnlock()
	if sps == nil {
		return "", nil
	}

	var rvgpuLayouts []rvgpuLayoutJson
	var safetyareas []safetyAreaJson
	found := false
	for _, ps := range sps.Pscreens {
		if ps.Rdisplay.RDisplayId != rId {
			continue
		}
		found = true
		for _, player := range ps.Players {
			layouts := genRvgpuLayoutParams(player, player.Psurfaces)
			addOrUpdateLayerSurfaces(rId, player.VID, player.Psurfaces)
			rvgpuLayouts = append(rvgpuLayouts, layouts...)
		}
		for _, r := range ps.PsafetyAreas {
			safetyareas = append(safetyareas, safetyAreaJson{
				X: int(r.PixelX), Y: int(r.PixelY),
				Width: int(r.PixelW), Height: int(r.PixelH),
			})
		}
	}
	if !found || len(rvgpuLayouts) == 0 {
		return "", nil
	}

	proto := InitialLayoutProtocol{
		Version:      VERSION,
		Command:      "initial_layout",
		RvgpuLayouts: rvgpuLayouts,
		SafetyAreas:  safetyareas,
	}
	jsonBytes, err := json.Marshal(proto)
	if err != nil {
		ELog.Println("buildInitialLayoutForDisplay marshal error:", err)
		return "", err
	}
	return string(jsonBytes), nil
}
