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

package layoutbackend

import (
	"unified-hmi/internal/layout/core"
)

func NewRdisplayCommandData(rdisp *layoutcore.RealDisplay, players []layoutcore.PixelLayer) (*RdisplayCommandData, error) {

	dcomm := RdisplayCommandData{
		Rdisplay: *rdisp.Dup(),
		Players:  layoutcore.DupPixelLayerSlice(players),
	}

	return &dcomm, nil
}

func NewRdisplayCommandDataWithPosition(rdisp *layoutcore.RealDisplay, order string, refId int,
	player layoutcore.PixelLayer) (*RdisplayCommandData, error) {

	players := make([]layoutcore.PixelLayer, 0)
	players = append(players, *player.Dup())

	dcomm := RdisplayCommandData{
		Rdisplay:    *rdisp.Dup(),
		InsertOrder: order,
		ReferenceId: refId,
		Players:     players,
	}

	return &dcomm, nil
}

func NewRdisplayCommandDataWithSafetyArea(rdisp *layoutcore.RealDisplay, players []layoutcore.PixelLayer, safetyareas []layoutcore.PixelSafetyArea) (*RdisplayCommandData, error) {

	dcomm := RdisplayCommandData{
		Rdisplay:    *rdisp.Dup(),
		Players:     layoutcore.DupPixelLayerSlice(players),
		SafetyAreas: layoutcore.DupPixelSafetyAreaSlice(safetyareas),
	}

	return &dcomm, nil
}

func NewEmptyLocalCommandReq() (*LocalCommandReq, error) {

	ltq := LocalCommandReq{}

	return &ltq, nil
}
