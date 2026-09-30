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
	"context"
	"fmt"
	"sort"

	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcore "unified-hmi/internal/layout/core"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

type layerGroupState struct {
	OriginalGroup string
	CurrentGroup  string
}

func (s *Server) sortLayersByPriorityConfig(layers []layoutcore.VirtualLayer) []layoutcore.VirtualLayer {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return layers
	}

	orderMap := make(map[layoutclusterapp.LayerKey]int)

	globalIdx := 0
	for _, g := range pc.PriorityGroups {
		for _, l := range g.Layers {
			key := layoutclusterapp.LayerKey{AppName: l.AppName, AreaName: l.AreaName}
			orderMap[key] = globalIdx
			globalIdx++
		}
	}

	sort.SliceStable(layers, func(i, j int) bool {
		keyI := layoutclusterapp.LayerKey{AppName: layers[i].AppName, AreaName: layers[i].AreaName}
		keyJ := layoutclusterapp.LayerKey{AppName: layers[j].AppName, AreaName: layers[j].AreaName}

		groupI := s.getCurrentGroup(keyI)
		groupJ := s.getCurrentGroup(keyJ)
		priI := pc.GetGroupPriority(groupI)
		priJ := pc.GetGroupPriority(groupJ)

		if priI != priJ {
			return priI < priJ
		}

		idxI, okI := orderMap[keyI]
		if !okI {
			idxI, okI = orderMap[layoutclusterapp.LayerKey{AppName: keyI.AppName}]
		}
		idxJ, okJ := orderMap[keyJ]
		if !okJ {
			idxJ, okJ = orderMap[layoutclusterapp.LayerKey{AppName: keyJ.AppName}]
		}
		if okI && okJ {
			return idxI < idxJ
		}
		if okI != okJ {
			return okI
		}
		if layers[i].ZOrder != layers[j].ZOrder {
			return layers[i].ZOrder < layers[j].ZOrder
		}
		if layers[i].VID != layers[j].VID {
			return layers[i].VID < layers[j].VID
		}
		if keyI.AppName != keyJ.AppName {
			return keyI.AppName < keyJ.AppName
		}
		return keyI.AreaName < keyJ.AreaName
	})
	return layers
}

func (s *Server) initLayerGroupStateLocked(pc *layoutclusterapp.PriorityConfig, key layoutclusterapp.LayerKey) {
	if pc == nil {
		return
	}
	group := pc.GetLayerGroup(key)
	s.layerGroupState[key] = &layerGroupState{
		OriginalGroup: group,
		CurrentGroup:  group,
	}
}

func (s *Server) initLayerGroupState(key layoutclusterapp.LayerKey) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return
	}
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	s.initLayerGroupStateLocked(pc, key)
}

func (s *Server) getCurrentGroup(key layoutclusterapp.LayerKey) string {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return ""
	}
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	state, ok := s.layerGroupState[key]
	if !ok {
		s.initLayerGroupStateLocked(pc, key)
		state = s.layerGroupState[key]
	}
	return state.CurrentGroup
}

func (s *Server) getOriginalGroup(key layoutclusterapp.LayerKey) string {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return ""
	}
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	state, ok := s.layerGroupState[key]
	if !ok {
		s.initLayerGroupStateLocked(pc, key)
		state = s.layerGroupState[key]
	}
	return state.OriginalGroup
}

func (s *Server) setCurrentGroup(key layoutclusterapp.LayerKey, group string) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return
	}
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	state, ok := s.layerGroupState[key]
	if !ok {
		s.initLayerGroupStateLocked(pc, key)
		state = s.layerGroupState[key]
	}
	state.CurrentGroup = group
}

func (s *Server) setGroupState(key layoutclusterapp.LayerKey, originalGroup, currentGroup string) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return
	}
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	state, ok := s.layerGroupState[key]
	if !ok {
		s.initLayerGroupStateLocked(pc, key)
		state = s.layerGroupState[key]
	}
	state.OriginalGroup = originalGroup
	state.CurrentGroup = currentGroup
}

func (s *Server) clearLayerGroupState() {
	s.layerGroupMu.Lock()
	defer s.layerGroupMu.Unlock()
	s.layerGroupState = make(map[layoutclusterapp.LayerKey]*layerGroupState)
}

func (s *Server) priorityEnabled() bool {
	return s.priorityConfig.Load() != nil
}

func (s *Server) initLayerGroupStateForLayers(layers []layoutcore.VirtualLayer) {
	if !s.priorityEnabled() {
		return
	}
	for _, layer := range layers {
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		s.initLayerGroupState(key)
	}
}

func (s *Server) reloadPriorityConfig() {
	pc, err := layoutclusterapp.ReadPriorityConfig()
	if err != nil {
		WLog.Printf("Priority config reload failed: %v", err)
		pc = nil
	}
	s.priorityConfig.Store(pc)
	s.clearLayerGroupState()
}

// ReloadPriorityConfig re-reads the priority policy (window_order_policy.json,
// or the Primary/General split derived from app-list-def.json when that file is
// absent) and re-seeds the group state of the layers already in the cache.
// Runtime SetAppPriorityGroup overrides are dropped, as with ApplySystemLayout.
func (s *Server) ReloadPriorityConfig() {
	s.reloadPriorityConfig()
	s.initLayerGroupStateForLayers(s.getAllCachedLayersSorted())
}

func (s *Server) checkGroupMoveAllowed(key layoutclusterapp.LayerKey, targetGroup string) bool {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return true
	}
	currentGroup := s.getCurrentGroup(key)
	if currentGroup == targetGroup {
		return true
	}

	currentPri := pc.GetGroupPriority(currentGroup)
	targetPri := pc.GetGroupPriority(targetGroup)

	if targetPri < currentPri {
		return true
	}

	if targetPri > currentPri {
		originalGroup := s.getOriginalGroup(key)
		if targetGroup == originalGroup {
			return true
		}
		return false
	}

	return true
}

func (s *Server) findInsertionRefInGroup(key layoutclusterapp.LayerKey, groupName string, existingLayers []layoutcore.VirtualLayer, selfVid int) (string, int) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return layoutcore.InsertAppend, -1
	}

	groupLayers := pc.GetGroupLayers(groupName)
	if groupLayers == nil {
		return layoutcore.InsertAppend, -1
	}

	myIdx := -1
	for i, gk := range groupLayers {
		if gk.AppName == key.AppName && (gk.AreaName == key.AreaName || (gk.AreaName == "" && key.AreaName == "")) {
			myIdx = i
			break
		}
	}

	if myIdx < 0 {
		return s.findGroupAppendRef(groupName, existingLayers, selfVid)
	}

	for i := myIdx - 1; i >= 0; i-- {
		precedingKey := groupLayers[i]
		if s.getCurrentGroup(precedingKey) != groupName {
			continue
		}
		for _, existing := range existingLayers {
			if existing.VID == selfVid {
				continue
			}
			if existing.AppName == precedingKey.AppName &&
				(precedingKey.AreaName == "" || existing.AreaName == precedingKey.AreaName) {
				return layoutcore.InsertAfter, existing.VID
			}
		}
	}

	return s.findGroupPrependRef(groupName, existingLayers, selfVid)
}

func (s *Server) findGroupAppendRef(groupName string, existingLayers []layoutcore.VirtualLayer, selfVid int) (string, int) {
	lastVid := -1
	for _, layer := range existingLayers {
		if layer.VID == selfVid {
			continue
		}
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		if s.getCurrentGroup(key) == groupName {
			lastVid = layer.VID
		}
	}
	if lastVid >= 0 {
		return layoutcore.InsertAfter, lastVid
	}

	return s.findGroupPrependRef(groupName, existingLayers, selfVid)
}

func (s *Server) findGroupPrependRef(groupName string, existingLayers []layoutcore.VirtualLayer, selfVid int) (string, int) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return layoutcore.InsertPrepend, -1
	}
	targetPri := pc.GetGroupPriority(groupName)
	if targetPri < 0 {
		return layoutcore.InsertPrepend, -1
	}

	lastVid := -1
	for _, layer := range existingLayers {
		if layer.VID == selfVid {
			continue
		}
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		layerGroup := s.getCurrentGroup(key)
		layerPri := pc.GetGroupPriority(layerGroup)
		if layerPri < targetPri {
			lastVid = layer.VID
		}
	}
	if lastVid >= 0 {
		return layoutcore.InsertAfter, lastVid
	}
	return layoutcore.InsertPrepend, -1
}

// ListPriorityGroups returns all priority groups defined in the active config
// together with per-layer detail for every layer currently in the layout cache.
// Each layer entry includes the live compositor z-order rank (from VScreen),
// current visibility, and destination rectangle so callers can see both the
// group membership and the actual on-screen display order at the same time.
// Layers within each group are sorted by z_rank ascending (0 = bottommost).
func (s *Server) ListPriorityGroups(ctx context.Context, req *uhmi.Empty) (*uhmi.ListPriorityGroupsResponse, error) {
	serverutil.LogFunc()
	pc := s.priorityConfig.Load()
	if pc == nil {
		return &uhmi.ListPriorityGroupsResponse{
			Status: "error",
			Info:   "priority groups not enabled",
		}, nil
	}

	layersByGroup := make(map[string][]*uhmi.PriorityGroupLayerInfo, len(pc.PriorityGroups))
	for _, g := range pc.PriorityGroups {
		layersByGroup[g.GroupName] = nil
	}

	for _, layer := range s.getAllCachedLayersSorted() {
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		group := s.getCurrentGroup(key)

		zRank := layoutvscreen.VScreen.GetVlayerZOrderRank(layer.VID)

		var visible bool
		var dstX, dstY, dstW, dstH float64
		if params, err := layoutvscreen.VScreen.GetVlayerParams(layer.VID); err == nil {
			visible = params.Visibility == nil || *params.Visibility != 0
			dstX, dstY, dstW, dstH = params.VdstX, params.VdstY, params.VdstW, params.VdstH
		}

		info := &uhmi.PriorityGroupLayerInfo{
			AppName:  layer.AppName,
			AreaName: layer.AreaName,
			Vid:      int32(layer.VID),
			ZRank:    int32(zRank),
			Visible:  visible,
			DstX:     dstX,
			DstY:     dstY,
			DstW:     dstW,
			DstH:     dstH,
		}

		if _, ok := layersByGroup[group]; !ok {
			layersByGroup[group] = nil
		}
		layersByGroup[group] = append(layersByGroup[group], info)
	}

	for g := range layersByGroup {
		layers := layersByGroup[g]
		sort.Slice(layers, func(i, j int) bool {
			return layers[i].ZRank < layers[j].ZRank
		})
	}

	entries := make([]*uhmi.PriorityGroupEntry, 0, len(pc.PriorityGroups))
	for _, g := range pc.PriorityGroups {
		entries = append(entries, &uhmi.PriorityGroupEntry{
			Name:      g.GroupName,
			Priority:  int32(pc.GetGroupPriority(g.GroupName)),
			IsDefault: g.GroupName == pc.DefaultGroup,
			Layers:    layersByGroup[g.GroupName],
		})
	}

	return &uhmi.ListPriorityGroupsResponse{
		Status: "success",
		Groups: entries,
	}, nil
}

// SetAppPriorityGroup permanently reassigns all layers of appName to the
// specified priority group and re-inserts them at the correct z-order position
// within that group. Both OriginalGroup and CurrentGroup are updated so that
// a subsequent "restore" also targets the new group.
func (s *Server) SetAppPriorityGroup(ctx context.Context, req *uhmi.SetAppPriorityGroupRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	targetGroup := req.GetGroup()
	ILog.Printf("[SetAppPriorityGroup] appName=%s group=%s", appName, targetGroup)

	if appName == "" {
		return &uhmi.Response{Status: "error", Info: "app_name is required"}, nil
	}
	pc := s.priorityConfig.Load()
	if pc == nil {
		return &uhmi.Response{Status: "error", Info: "priority groups not enabled"}, nil
	}
	if pc.GetGroupPriority(targetGroup) < 0 {
		return &uhmi.Response{Status: "error", Info: "unknown group: " + targetGroup}, nil
	}

	layers, found := s.GetCachedAppLayout(appName)
	if !found || len(layers) == 0 {
		return &uhmi.Response{Status: "error", Info: "app not found in layout cache: " + appName}, nil
	}

	for _, layer := range layers {
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		s.setGroupState(key, targetGroup, targetGroup)
	}

	for _, layer := range layers {
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		s.removeWindowOrderConstraint(key)
	}
	if err := s.publishDesiredOrder(); err != nil {
		return &uhmi.Response{Status: "error", Info: err.Error()}, nil
	}

	s.logLayerHierarchy()
	return &uhmi.Response{Status: "success", Info: fmt.Sprintf("app %s moved to group %s", appName, targetGroup)}, nil
}
