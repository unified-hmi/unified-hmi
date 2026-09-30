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

package layoutserver

import (
	"fmt"

	layoutparams "unified-hmi/internal/layout/params"
)

// MirrorEntry records the original app and the area name assigned to a mirror.
// The mirrorCacheKey (e.g. "appName_mirror0") is used as the layout-cache key;
// the VirtualLayer itself carries AppName=OriginalAppName and AreaName so the
// compositor sees the layer as an area of the original app.
type MirrorEntry struct {
	OriginalAppName string
	AreaName        string // e.g. "mirror0"
}

func (s *Server) AllocMirrorName(originalAppName string) (name string, n int) {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	idx := s.mirrorIndex[originalAppName]
	s.mirrorIndex[originalAppName] = idx + 1
	return fmt.Sprintf("%s_mirror%d", originalAppName, idx), idx + 1
}

func (s *Server) IsMirror(name string) bool {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	_, ok := s.mirrorMap[name]
	return ok
}

// RegisterMirror records the mirror in the registry and registers two dynamic
// VID mappings so that GetVIDFromDrawAreas resolves the mirror layer for all
// callers regardless of how they address it:
//
//   - (originalAppName, areaName) → vid  used by internal paths that track the
//     layer under the original app key (resolveMirrorLayerKey, group state, etc.)
//   - (mirrorName, "")          → vid  used by external callers that address the
//     mirror by the name returned from CreateAppMirror (MoveWindow,
//     StartSystemAnimation, etc.)
func (s *Server) RegisterMirror(mirrorName, originalAppName, areaName string, vid int) {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	s.mirrorMap[mirrorName] = MirrorEntry{OriginalAppName: originalAppName, AreaName: areaName}
	layoutparams.RegisterDynamicVID(originalAppName, areaName, vid)
	layoutparams.RegisterDynamicVID(mirrorName, "", vid)
}

func (s *Server) UnregisterMirror(mirrorName string) {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	if entry, ok := s.mirrorMap[mirrorName]; ok {
		layoutparams.UnregisterDynamicVID(entry.OriginalAppName, entry.AreaName)
		layoutparams.UnregisterDynamicVID(mirrorName, "")
	}
	delete(s.mirrorMap, mirrorName)
}

// UnregisterMirrorsFor removes all mirrors of originalAppName from mirrorMap
// and resets its index counter. Returns the mirror names that were removed.
func (s *Server) UnregisterMirrorsFor(originalAppName string) []string {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	var removed []string
	for mirrorName, entry := range s.mirrorMap {
		if entry.OriginalAppName == originalAppName {
			removed = append(removed, mirrorName)
			layoutparams.UnregisterDynamicVID(entry.OriginalAppName, entry.AreaName)
			layoutparams.UnregisterDynamicVID(mirrorName, "")
		}
	}
	for _, m := range removed {
		delete(s.mirrorMap, m)
	}
	delete(s.mirrorIndex, originalAppName)
	return removed
}

// DrainAllMirrors removes all entries from mirrorMap and mirrorIndex and
// returns the mirror names that were registered.
func (s *Server) DrainAllMirrors() []string {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	names := make([]string, 0, len(s.mirrorMap))
	for name, entry := range s.mirrorMap {
		names = append(names, name)
		layoutparams.UnregisterDynamicVID(entry.OriginalAppName, entry.AreaName)
		layoutparams.UnregisterDynamicVID(name, "")
	}
	s.mirrorMap = make(map[string]MirrorEntry)
	s.mirrorIndex = make(map[string]int)
	return names
}

func (s *Server) GetMirrorNames() []string {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	names := make([]string, 0, len(s.mirrorMap))
	for name := range s.mirrorMap {
		names = append(names, name)
	}
	return names
}

func (s *Server) resolveMirrorLayerKey(appName string) (string, string) {
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	if entry, ok := s.mirrorMap[appName]; ok {
		return entry.OriginalAppName, entry.AreaName
	}
	return appName, ""
}
