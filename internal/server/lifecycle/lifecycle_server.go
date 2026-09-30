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

// Package lifecycleserver implements the LIFECYCLE domain of the unified
// UHMIService server: app run/stop/list and the node/receiver
// distribution used to launch apps across worker nodes.
package lifecycleserver

import (
	"unified-hmi/internal/config"
)

var gVScrnDef *config.VScrnDef = nil

// Server implements the LIFECYCLE domain handlers. Its concurrent registries
// (comm tasks, receiver refs) are package-level, so Server itself only
// carries the shared virtual-screen definition.
type Server struct{}

// NewServer creates the LIFECYCLE domain server and stores the shared
// virtual-screen definition used for node/command resolution.
func NewServer(vscrnDef *config.VScrnDef) *Server {
	gVScrnDef = vscrnDef
	return &Server{}
}
