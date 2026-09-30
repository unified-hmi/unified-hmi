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
	"sort"

	layoutcommgen "unified-hmi/internal/layout/commgen"
	layoutcore "unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

func (s *Server) cacheIsEmpty() bool {
	s.layoutCacheMu.RLock()
	defer s.layoutCacheMu.RUnlock()
	return len(s.layoutCacheByApp) == 0
}

func (s *Server) GetCachedAppLayout(appName string) ([]layoutcore.VirtualLayer, bool) {
	s.layoutCacheMu.RLock()
	defer s.layoutCacheMu.RUnlock()
	layers, ok := s.layoutCacheByApp[appName]
	if !ok {
		return nil, false
	}
	return dupVirtualLayerSlice(layers), true
}

func (s *Server) setCachedAppLayout(appName string, layers []layoutcore.VirtualLayer) {
	s.layoutCacheMu.Lock()
	defer s.layoutCacheMu.Unlock()
	s.layoutCacheByApp[appName] = dupVirtualLayerSlice(layers)
}

func (s *Server) deleteCachedAppLayout(appName string) {
	s.layoutCacheMu.Lock()
	defer s.layoutCacheMu.Unlock()
	delete(s.layoutCacheByApp, appName)
}

func (s *Server) clearLayoutCache() {
	s.layoutCacheMu.Lock()
	defer s.layoutCacheMu.Unlock()
	s.layoutCacheByApp = make(map[string][]layoutcore.VirtualLayer)
}

// PurgeLayoutCache drops every cached app layout together with the derived
// ordering state (priority-group assignments and window-order constraints).
// It is used when the whole system is torn down (StopAll) so that stale
// entries — e.g. registered via SetLayoutCommand with a wrong
// application_name for an app that never actually ran — cannot keep
// corrupting the layer add/insert logic of later launches. After a purge the
// next layout application takes the cache-empty path and rebuilds the scene
// with initial_vscreen.
func (s *Server) PurgeLayoutCache() {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()

	s.clearLayoutCache()
	s.clearLayerGroupState()
	s.clearWindowOrderState()
	ILog.Printf("[PurgeLayoutCache] layout cache and ordering state cleared")
}

func (s *Server) rebuildLayoutCache(ctree *layoutcore.LayoutTree) {
	next := make(map[string][]layoutcore.VirtualLayer)
	for _, layer := range ctree.Vlayers {
		next[layer.AppName] = append(next[layer.AppName], *layer.Dup())
	}

	s.layoutCacheMu.Lock()
	defer s.layoutCacheMu.Unlock()
	s.layoutCacheByApp = next
}

func (s *Server) getAllCachedLayersSorted() []layoutcore.VirtualLayer {
	s.layoutCacheMu.RLock()
	defer s.layoutCacheMu.RUnlock()
	all := make([]layoutcore.VirtualLayer, 0)
	for _, layers := range s.layoutCacheByApp {
		for _, layer := range layers {
			all = append(all, *layer.Dup())
		}
	}
	if s.priorityConfig.Load() != nil {
		all = s.sortLayersByPriorityConfig(all)
	} else {
		sort.Slice(all, func(i, j int) bool {
			return all[i].ZOrder < all[j].ZOrder
		})
	}
	return all
}

// BuildInitialVscreenSnapshot serialises the currently-cached intended scene
// (all apps, sorted by priority/z-order) as an initial_vscreen JSON command.
// Returns ("", nil) when the cache is empty. Suitable for pushing to a worker
// that just (re)connected; nodeId is unused today but reserved for future
// per-node scoping.
func (s *Server) BuildInitialVscreenSnapshot(nodeId int) (string, error) {
	layers := s.getAllCachedLayersSorted()
	if len(layers) == 0 {
		return "", nil
	}
	tree := &layoutcore.LayoutTree{Vlayers: layers}
	return layoutcommgen.GenerateCommInitialVscreen(tree)
}
