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

type RdisplayCommandData struct {
	Rdisplay    layoutcore.RealDisplay
	InsertOrder string
	ReferenceId int
	Players     []layoutcore.PixelLayer
	SafetyAreas []layoutcore.PixelSafetyArea
}

type LocalCommandReq struct {
	Command string
	RDComms []RdisplayCommandData
	Ret     int
}

type LocalCommandGenerator interface {
	Start(reqChan chan LocalCommandReq, respChan chan LocalCommandReq)
	GenerateLocalCommandReq(*layoutcore.ApplyCommandData, *layoutcore.NodePixelScreens) ([]*LocalCommandReq, error)
}
