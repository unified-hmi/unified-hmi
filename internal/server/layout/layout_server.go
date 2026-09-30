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

// Package layoutserver implements the LAYOUT domain of the unified UHMIService
// server: window/layer cache, priority groups, mirroring, animation and
// notification handling. Every RPC-facing method is exported on *Server so
// uhmiserver can delegate to it directly.
package layoutserver

import (
	"sync"

	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcore "unified-hmi/internal/layout/core"
	layoutmulticonn "unified-hmi/internal/layout/multiconn"
	. "unified-hmi/internal/ulog"
)

// Server holds the LAYOUT domain state: the per-app layer cache, the active
// priority configuration, per-layer group assignment and the mirror registry.
type Server struct {
	layoutMutationMu sync.Mutex

	layoutCacheMu     sync.RWMutex
	layoutCacheByApp  map[string][]layoutcore.VirtualLayer
	sendLayoutCommand func(string) error

	priorityConfig layoutclusterapp.PriorityConfigHolder

	layerGroupMu    sync.Mutex
	layerGroupState map[layoutclusterapp.LayerKey]*layerGroupState

	orderMu          sync.Mutex
	orderConstraints map[layoutclusterapp.LayerKey]windowOrderConstraint
	orderSnapshot    []layoutclusterapp.LayerKey
	orderSeq         uint64

	mirrorMu    sync.Mutex
	mirrorMap   map[string]MirrorEntry
	mirrorIndex map[string]int
}

// NewServer creates the LAYOUT domain server and loads the priority config
// (a load failure just disables priority groups; it is not fatal).
func NewServer() *Server {
	srv := &Server{
		layoutCacheByApp: make(map[string][]layoutcore.VirtualLayer),
		sendLayoutCommand: func(command string) error {
			return layoutmulticonn.MulCon.SendLayoutCommand(command)
		},
		layerGroupState:  make(map[layoutclusterapp.LayerKey]*layerGroupState),
		orderConstraints: make(map[layoutclusterapp.LayerKey]windowOrderConstraint),
		mirrorMap:        make(map[string]MirrorEntry),
		mirrorIndex:      make(map[string]int),
	}
	pc, err := layoutclusterapp.ReadPriorityConfig()
	if err != nil {
		WLog.Printf("Priority config not loaded, priority groups disabled: %v", err)
	} else {
		srv.priorityConfig.Store(pc)
	}
	layoutmulticonn.SetResyncProvider(srv.BuildInitialVscreenSnapshot)
	return srv
}
