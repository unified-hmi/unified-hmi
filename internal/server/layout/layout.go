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
	"encoding/json"
	"fmt"
	"sort"

	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcommgen "unified-hmi/internal/layout/commgen"
	layoutcore "unified-hmi/internal/layout/core"
	layoutmulticonn "unified-hmi/internal/layout/multiconn"
	layoutparams "unified-hmi/internal/layout/params"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func failResponse(op string, err error) (*uhmi.Response, error) {
	ELog.Println(err)
	return &uhmi.Response{Status: "Failed to Layout" + op}, err
}

func (s *Server) getAllLayersWithCurrentPos() []layoutcore.VirtualLayer {
	cached := s.getAllCachedLayersSorted()
	result := make([]layoutcore.VirtualLayer, 0, len(cached))
	for _, layer := range cached {
		current, err := layoutvscreen.VScreen.GetVlayerParams(layer.VID)
		if err != nil {
			result = append(result, layer)
			continue
		}
		current.AppName = layer.AppName
		current.AreaName = layer.AreaName
		current.GroupName = layer.GroupName
		result = append(result, current)
	}
	return result
}

func (s *Server) reorderBeforeMove(movedVID int, movedAppName string, dstVlayer layoutcore.VirtualLayer) error {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return nil
	}

	resolvedApp, resolvedArea := s.resolveMirrorLayerKey(movedAppName)
	movedKey := layoutclusterapp.LayerKey{AppName: resolvedApp, AreaName: resolvedArea}
	movedGroup := s.getCurrentGroup(movedKey)
	movedPri := pc.GetGroupPriority(movedGroup)
	dstX, dstY, dstW, dstH := dstVlayer.VdstX, dstVlayer.VdstY, dstVlayer.VdstW, dstVlayer.VdstH

	allLayers := s.getAllLayersWithCurrentPos()

	var higherGroup, sameGroup, lowerGroup []layoutcore.VirtualLayer
	for _, layer := range allLayers {
		if layer.VID == movedVID {
			continue
		}
		if !layoutcore.LayersOverlap(dstX, dstY, dstW, dstH, layer.VdstX, layer.VdstY, layer.VdstW, layer.VdstH) {
			continue
		}
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		layerPri := pc.GetGroupPriority(s.getCurrentGroup(key))
		switch {
		case layerPri > movedPri:
			higherGroup = append(higherGroup, layer)
		case layerPri == movedPri:
			sameGroup = append(sameGroup, layer)
		default:
			lowerGroup = append(lowerGroup, layer)
		}
	}

	if len(higherGroup) == 0 && len(sameGroup) == 0 && len(lowerGroup) == 0 {
		DLog.Printf("[reorderBeforeMove] %s: no overlapping layers at dst=(%.0f,%.0f,%.0f,%.0f)",
			movedAppName, dstX, dstY, dstW, dstH)
		return nil
	}

	zRank := func(vid int) int { return layoutvscreen.VScreen.GetVlayerZOrderRank(vid) }
	sort.Slice(higherGroup, func(i, j int) bool { return zRank(higherGroup[i].VID) < zRank(higherGroup[j].VID) })
	sort.Slice(sameGroup, func(i, j int) bool { return zRank(sameGroup[i].VID) < zRank(sameGroup[j].VID) })
	sort.Slice(lowerGroup, func(i, j int) bool { return zRank(lowerGroup[i].VID) < zRank(lowerGroup[j].VID) })

	var order string
	var refVid int
	if len(higherGroup) > 0 {
		order, refVid = layoutcore.InsertBefore, higherGroup[0].VID
	} else if len(sameGroup) > 0 {
		order, refVid = layoutcore.InsertAfter, sameGroup[len(sameGroup)-1].VID
	} else {
		order, refVid = layoutcore.InsertAfter, lowerGroup[len(lowerGroup)-1].VID
	}

	movedVlayer, err := layoutvscreen.VScreen.GetVlayerParams(movedVID)
	if err != nil {
		return fmt.Errorf("reorderBeforeMove: GetVlayerParams(%d): %w", movedVID, err)
	}

	layoutComm, err := layoutcommgen.GenerateCommAddVlayer(movedVlayer, order, refVid)
	if err != nil {
		return err
	}
	if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
		return err
	}
	ILog.Printf("[reorderBeforeMove] %s (group=%s pri=%d) → %s refVid=%d dst=(%.0f,%.0f,%.0f,%.0f)",
		movedAppName, movedGroup, movedPri, order, refVid, dstX, dstY, dstW, dstH)
	s.logLayerHierarchy()
	return nil
}

func (s *Server) logLayerHierarchy() {
	allLayers := s.getAllCachedLayersSorted()
	if len(allLayers) == 0 {
		DLog.Println("--- Layer Hierarchy (empty) ---")
		return
	}

	var lines []string
	lines = append(lines, "--- Layer Hierarchy ---")

	if pc := s.priorityConfig.Load(); pc != nil {
		groupLayers := make(map[string][]layoutcore.VirtualLayer)
		for _, layer := range allLayers {
			key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
			group := s.getCurrentGroup(key)
			groupLayers[group] = append(groupLayers[group], layer)
		}

		for _, g := range pc.PriorityGroups {
			lines = append(lines, g.GroupName)
			layers := groupLayers[g.GroupName]
			for i, layer := range layers {
				prefix := "  |_"
				if i == 0 {
					prefix = "  `-"
				}
				lines = append(lines, fmt.Sprintf("%s%s:%s", prefix, layer.AppName, layer.AreaName))
			}
		}
	} else {
		lines = append(lines, "(no priority groups)")
		for i, layer := range allLayers {
			prefix := "  |_"
			if i == 0 {
				prefix = "  `-"
			}
			lines = append(lines, fmt.Sprintf("%s%s:%s (z=%d)", prefix, layer.AppName, layer.AreaName, layer.ZOrder))
		}
	}

	lines = append(lines, "-----------------------")
	for _, line := range lines {
		DLog.Println(line)
	}
}

func (s *Server) ApplyWindowOrder(vid int, appName string, areaName string, windowOrder *layoutcore.WindowOrderSetting) error {
	var vlayer layoutcore.VirtualLayer
	var err error
	vlayer, err = layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return err
	}

	layerKey := layoutclusterapp.LayerKey{AppName: appName, AreaName: areaName}

	if s.priorityConfig.Load() == nil {
		if windowOrder == nil {
			return nil
		}
		refVid := -1
		if windowOrder.Order == layoutcore.InsertBefore || windowOrder.Order == layoutcore.InsertAfter {
			refVid, err = layoutparams.GetVIDFromDrawAreas(windowOrder.RefAppName, windowOrder.RefArea)
			if err != nil {
				return err
			}
		}
		layoutComm, err := layoutcommgen.GenerateCommAddVlayer(vlayer, windowOrder.Order, refVid)
		if err != nil {
			return err
		}
		return layoutmulticonn.MulCon.SendLayoutCommand(layoutComm)
	}

	if windowOrder == nil {
		originalGroup := s.getOriginalGroup(layerKey)
		if s.getCurrentGroup(layerKey) != originalGroup {
			s.setCurrentGroup(layerKey, originalGroup)
		}
		s.removeWindowOrderConstraint(layerKey)
		return s.publishDesiredOrder()
	}

	constraint := windowOrderConstraint{Order: windowOrder.Order}

	if windowOrder.Order == layoutcore.InsertBefore || windowOrder.Order == layoutcore.InsertAfter {
		refKey := layoutclusterapp.LayerKey{AppName: windowOrder.RefAppName, AreaName: windowOrder.RefArea}
		refGroup := s.getCurrentGroup(refKey)
		if refGroup != s.getCurrentGroup(layerKey) {
			if !s.checkGroupMoveAllowed(layerKey, refGroup) {
				WLog.Printf("Animation window order denied: cannot move %s/%s to group %s",
					appName, areaName, refGroup)
				return nil
			}
			s.setCurrentGroup(layerKey, refGroup)
		}
		constraint.RefKey = refKey
	}

	s.setWindowOrderConstraint(layerKey, constraint)

	if err := s.publishDesiredOrder(); err != nil {
		return err
	}
	s.logLayerHierarchy()
	return nil
}

func (s *Server) assignGroupInfoToLayers(tree *layoutcore.LayoutTree) {
	pc := s.priorityConfig.Load()
	if pc == nil {
		return
	}
	for i := range tree.Vlayers {
		layer := &tree.Vlayers[i]
		if layer.AreaName == "" {
			layer.AreaName = layoutclusterapp.LookupAreaNameByVID(layer.AppName, layer.VID)
		}
		key := layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
		layer.GroupName = pc.GetLayerGroup(key)
	}
}

func (s *Server) sendSingleAppAsInitialVscreen(layout *layoutcore.LayoutTree) error {
	layoutComm, err := layoutcommgen.GenerateCommInitialVscreen(layout)
	if err != nil {
		return err
	}
	ILog.Printf("[sendSingleAppAsInitialVscreen] command=%s", layoutComm)
	return s.sendLayoutCommand(layoutComm)
}

func (s *Server) sendSingleAppAsAddCommands(layers []layoutcore.VirtualLayer, existingLayers []layoutcore.VirtualLayer) error {
	ILog.Printf("[sendSingleAppAsAddCommands] adding %d layers, existing %d layers", len(layers), len(existingLayers))
	for _, cLayer := range layers {
		ILog.Printf("[sendSingleAppAsAddCommands] adding VID=%d AppName=%s AreaName=%s", cLayer.VID, cLayer.AppName, cLayer.AreaName)

		order := layoutcore.InsertAppend
		refVid := -1

		if s.priorityConfig.Load() != nil {
			key := layoutclusterapp.LayerKey{AppName: cLayer.AppName, AreaName: cLayer.AreaName}
			groupName := s.getCurrentGroup(key)
			order, refVid = s.findInsertionRefInGroup(key, groupName, existingLayers, cLayer.VID)
			ILog.Printf("[sendSingleAppAsAddCommands] group=%s order=%s refVid=%d", groupName, order, refVid)
		} else {
			for _, existing := range existingLayers {
				if existing.ZOrder > cLayer.ZOrder {
					order = layoutcore.InsertBefore
					refVid = existing.VID
					break
				}
			}
		}

		layoutComm, err := layoutcommgen.GenerateCommAddVlayer(cLayer, order, refVid)
		if err != nil {
			return err
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return err
		}
		for _, vSurface := range cLayer.Vsurfaces {
			surfComm, err := layoutcommgen.GenerateCommAddVsurface(vSurface, layoutcore.InsertAppend, -1, cLayer.VID)
			if err != nil {
				return err
			}
			if err := layoutmulticonn.MulCon.SendLayoutCommand(surfComm); err != nil {
				return err
			}
		}

		if s.priorityConfig.Load() != nil {
			if order == layoutcore.InsertPrepend {
				existingLayers = append([]layoutcore.VirtualLayer{cLayer}, existingLayers...)
			} else if order == layoutcore.InsertAfter && refVid >= 0 {
				for idx, existing := range existingLayers {
					if existing.VID == refVid {
						existingLayers = append(existingLayers[:idx+1], append([]layoutcore.VirtualLayer{cLayer}, existingLayers[idx+1:]...)...)
						break
					}
				}
			} else if order == layoutcore.InsertBefore && refVid >= 0 {
				for idx, existing := range existingLayers {
					if existing.VID == refVid {
						existingLayers = append(existingLayers[:idx], append([]layoutcore.VirtualLayer{cLayer}, existingLayers[idx:]...)...)
						break
					}
				}
			} else {
				existingLayers = append(existingLayers, cLayer)
			}
		} else {
			inserted := false
			for idx, existing := range existingLayers {
				if existing.ZOrder > cLayer.ZOrder {
					existingLayers = append(existingLayers[:idx+1], existingLayers[idx:]...)
					existingLayers[idx] = cLayer
					inserted = true
					break
				}
			}
			if !inserted {
				existingLayers = append(existingLayers, cLayer)
			}
		}
	}
	return nil
}

func sendSingleAppAsModifyCommands(layers []layoutcore.VirtualLayer) error {
	ILog.Printf("[sendSingleAppAsModifyCommands] modifying %d layers", len(layers))
	for _, cLayer := range layers {
		ILog.Printf("[sendSingleAppAsModifyCommands] modifying VID=%d AppName=%s", cLayer.VID, cLayer.AppName)
		layoutComm, err := layoutcommgen.GenerateCommModifyVlayer(cLayer)
		if err != nil {
			return err
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return err
		}
		for _, vSurface := range cLayer.Vsurfaces {
			var surfLayer layoutcore.VirtualLayer
			surfLayer.VID = cLayer.VID
			surfLayer.Vsurfaces = []layoutcore.VirtualSurface{vSurface}
			surfComm, err := layoutcommgen.GenerateCommModifyVsurface(surfLayer)
			if err != nil {
				return err
			}
			if err := layoutmulticonn.MulCon.SendLayoutCommand(surfComm); err != nil {
				return err
			}
		}
	}
	return nil
}

func sendSingleAppAsRemoveCommands(layerIDs []int) error {
	for _, layerID := range layerIDs {
		vLayer := layoutcore.VirtualLayer{}
		vLayer.VID = layerID
		layoutComm, err := layoutcommgen.GenerateCommRemoveVlayer(vLayer)
		if err != nil {
			return err
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) ApplySystemLayout(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	serverutil.LogFunc()
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()

	s.clearLayoutCache()
	s.clearWindowOrderState()
	s.reloadPriorityConfig()

	layoutTree, err := layoutclusterapp.ReadLayoutTreeFromCfg()
	if err != nil {
		return &uhmi.Response{Status: "Failed to ApplySystemLayout"}, err
	}

	if s.priorityEnabled() {
		s.assignGroupInfoToLayers(layoutTree)
		layoutTree.Vlayers = s.sortLayersByPriorityConfig(layoutTree.Vlayers)
	}

	var layoutComm string
	layoutComm, err = layoutcommgen.GenerateCommInitialVscreen(layoutTree)
	if err != nil {
		return &uhmi.Response{Status: "Failed to ApplySystemLayout"}, err
	}

	err = layoutmulticonn.MulCon.SendLayoutCommand(layoutComm)
	if err != nil {
		return &uhmi.Response{Status: "Failed to ApplySystemLayout"}, err
	}

	s.rebuildLayoutCache(layoutTree)
	s.seedWindowOrderSnapshot(layoutTree.Vlayers)

	s.initLayerGroupStateForLayers(layoutTree.Vlayers)

	s.logLayerHierarchy()
	return &uhmi.Response{Status: "System layout set successfully"}, nil
}

func (s *Server) applyAppLayoutTree(appName string, appTree *layoutcore.LayoutTree, op, successStatus string) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()

	if appTree == nil {
		return &uhmi.Response{Status: "Failed to " + op}, fmt.Errorf("appTree is nil")
	}
	if s.priorityEnabled() {
		s.assignGroupInfoToLayers(appTree)
	}
	newLayers := dupVirtualLayerSlice(appTree.Vlayers)
	if s.priorityEnabled() {
		newLayers = s.sortLayersByPriorityConfig(newLayers)
	}
	if s.cacheIsEmpty() {
		if err := s.sendSingleAppAsInitialVscreen(appTree); err != nil {
			return failResponse(op, err)
		}
		s.setCachedAppLayout(appName, newLayers)
		s.initLayerGroupStateForLayers(newLayers)
		s.logLayerHierarchy()
		return &uhmi.Response{Status: successStatus}, nil
	}
	oldLayers, found := s.GetCachedAppLayout(appName)
	if !found {
		existingLayers := s.getAllCachedLayersSorted()
		if err := s.sendSingleAppAsAddCommands(newLayers, existingLayers); err != nil {
			return failResponse(op, err)
		}
		s.setCachedAppLayout(appName, newLayers)
		s.initLayerGroupStateForLayers(newLayers)
		s.logLayerHierarchy()
		return &uhmi.Response{Status: successStatus}, nil
	}
	if resp, err := s.applyLayerDiff(oldLayers, newLayers, "Failed to "+op); err != nil {
		return resp, err
	}
	s.setCachedAppLayout(appName, newLayers)
	s.logLayerHierarchy()
	return &uhmi.Response{Status: successStatus}, nil
}

func isWireProtocolCommand(mJson map[string]interface{}) bool {
	_, ok := mJson["command"]
	return ok
}

// ReSyncSystemLayout pushes a no-op LAYOUT command to every worker so that any
// freshly-reconnected compositor is caught up via replayReconnected. No
// change is made to the scene state.
func (s *Server) ReSyncSystemLayout(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	serverutil.LogFunc()

	cmd, err := layoutcommgen.GenerateCommResync()
	if err != nil {
		return &uhmi.Response{Status: "Failed to ReSyncSystemLayout"}, err
	}
	if err := layoutmulticonn.MulCon.SendLayoutCommand(cmd); err != nil {
		return &uhmi.Response{Status: "Failed to ReSyncSystemLayout"}, err
	}
	return &uhmi.Response{Status: "ReSync system layout successfully"}, nil
}

func (s *Server) applyLayerDiff(oldLayers, newLayers []layoutcore.VirtualLayer, failStatus string) (*uhmi.Response, error) {
	removeIDs, addLayers, modifyLayers := layoutcore.DiffLayersByVID(oldLayers, newLayers)
	if err := sendSingleAppAsRemoveCommands(removeIDs); err != nil {
		return &uhmi.Response{Status: failStatus}, err
	}
	if err := s.sendSingleAppAsAddCommands(addLayers, s.getAllCachedLayersSorted()); err != nil {
		return &uhmi.Response{Status: failStatus}, err
	}
	if err := sendSingleAppAsModifyCommands(modifyLayers); err != nil {
		return &uhmi.Response{Status: failStatus}, err
	}
	return nil, nil
}

// ApplyApplicationLayout applies a pre-built LayoutTree for appName directly,
// without reading any layout JSON files from disk.  It follows the same cache
// update logic as SetApplicationLayout.
func (s *Server) ApplyApplicationLayout(appName string, appTree *layoutcore.LayoutTree) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()

	if appName == "" {
		return &uhmi.Response{Status: "Failed to LayoutApplyApplicationLayout: app_name is required"}, fmt.Errorf("app_name is required")
	}
	if s.priorityEnabled() {
		s.assignGroupInfoToLayers(appTree)
	}
	newLayers := dupVirtualLayerSlice(appTree.Vlayers)
	if s.priorityEnabled() {
		newLayers = s.sortLayersByPriorityConfig(newLayers)
	}
	if s.cacheIsEmpty() {
		if err := s.sendSingleAppAsInitialVscreen(appTree); err != nil {
			return &uhmi.Response{Status: "Failed to LayoutApplyApplicationLayout"}, err
		}
		s.setCachedAppLayout(appName, newLayers)
		s.initLayerGroupStateForLayers(newLayers)
		s.logLayerHierarchy()
		return &uhmi.Response{Status: "Application layout applied successfully"}, nil
	}
	oldLayers, found := s.GetCachedAppLayout(appName)
	if !found {
		existingLayers := s.getAllCachedLayersSorted()
		if err := s.sendSingleAppAsAddCommands(newLayers, existingLayers); err != nil {
			return &uhmi.Response{Status: "Failed to LayoutApplyApplicationLayout"}, err
		}
		s.setCachedAppLayout(appName, newLayers)
		s.initLayerGroupStateForLayers(newLayers)
		s.logLayerHierarchy()
		return &uhmi.Response{Status: "Application layout applied successfully"}, nil
	}
	if resp, err := s.applyLayerDiff(oldLayers, newLayers, "Failed to LayoutApplyApplicationLayout"); err != nil {
		return resp, err
	}
	s.setCachedAppLayout(appName, newLayers)
	s.logLayerHierarchy()
	return &uhmi.Response{Status: "Application layout applied successfully"}, nil
}

// ApplyMultiAppEntry holds the appName and in-memory LayoutTree for a single
// application, used as input to ApplyMultipleApplicationLayouts.
type ApplyMultiAppEntry struct {
	AppName string
	Tree    *layoutcore.LayoutTree
}

// ApplyMultipleApplicationLayouts applies layout trees for multiple apps in a
// single initial_vscreen command, mirroring ApplySystemLayout's approach.
// All new app layers are merged with any existing cached layers (from apps not
// in the batch) and sent as one atomic initial_vscreen.  This avoids the TOCTOU
// race that arises when concurrent ApplyApplicationLayout calls each see an
// empty cache and independently emit initial_vscreen.
func (s *Server) ApplyMultipleApplicationLayouts(entries []ApplyMultiAppEntry) (*uhmi.Response, error) {
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()

	if len(entries) == 0 {
		return &uhmi.Response{Status: "ApplyMultipleApplicationLayouts: no entries"}, nil
	}

	newLayersByApp := make(map[string][]layoutcore.VirtualLayer, len(entries))
	for _, e := range entries {
		if s.priorityEnabled() {
			s.assignGroupInfoToLayers(e.Tree)
		}
		layers := dupVirtualLayerSlice(e.Tree.Vlayers)
		if s.priorityEnabled() {
			layers = s.sortLayersByPriorityConfig(layers)
		}
		newLayersByApp[e.AppName] = layers
	}

	if s.cacheIsEmpty() {
		allLayers := make([]layoutcore.VirtualLayer, 0)
		for _, e := range entries {
			allLayers = append(allLayers, newLayersByApp[e.AppName]...)
		}
		if s.priorityEnabled() {
			allLayers = s.sortLayersByPriorityConfig(allLayers)
		} else {
			sort.Slice(allLayers, func(i, j int) bool {
				return allLayers[i].ZOrder < allLayers[j].ZOrder
			})
		}
		combined := &layoutcore.LayoutTree{Vlayers: allLayers}
		layoutComm, err := layoutcommgen.GenerateCommInitialVscreen(combined)
		if err != nil {
			return &uhmi.Response{Status: "Failed to LayoutApplyMultipleApplicationLayouts"}, err
		}
		ILog.Printf("[ApplyMultipleApplicationLayouts] cache empty: initial_vscreen apps=%d layers=%d", len(entries), len(allLayers))
		if err := layoutmulticonn.MulCon.SendLayoutCommand(layoutComm); err != nil {
			return &uhmi.Response{Status: "Failed to LayoutApplyMultipleApplicationLayouts"}, err
		}
	} else {
		existingLayers := s.getAllCachedLayersSorted()
		ILog.Printf("[ApplyMultipleApplicationLayouts] cache non-empty: add_vlayer apps=%d existingLayers=%d", len(entries), len(existingLayers))
		for _, e := range entries {
			layers := newLayersByApp[e.AppName]
			if err := s.sendSingleAppAsAddCommands(layers, existingLayers); err != nil {
				return &uhmi.Response{Status: "Failed to LayoutApplyMultipleApplicationLayouts"}, err
			}
			existingLayers = append(existingLayers, layers...)
			if s.priorityEnabled() {
				existingLayers = s.sortLayersByPriorityConfig(existingLayers)
			} else {
				sort.Slice(existingLayers, func(i, j int) bool {
					return existingLayers[i].ZOrder < existingLayers[j].ZOrder
				})
			}
		}
	}

	for _, e := range entries {
		layers := newLayersByApp[e.AppName]
		s.setCachedAppLayout(e.AppName, layers)
		s.initLayerGroupStateForLayers(layers)
	}

	s.logLayerHierarchy()
	return &uhmi.Response{Status: "Multiple application layouts applied successfully"}, nil
}

func (s *Server) SetApplicationLayout(ctx context.Context, req *uhmi.SetApplicationLayoutRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()

	if appName == "" {
		return &uhmi.Response{Status: "Failed to LayoutSetApplicationLayout: app_name is required"}, fmt.Errorf("app_name is required")
	}

	s.layoutCacheMu.RLock()
	cacheLen := len(s.layoutCacheByApp)
	cacheKeys := make([]string, 0, cacheLen)
	for k := range s.layoutCacheByApp {
		cacheKeys = append(cacheKeys, k)
	}
	s.layoutCacheMu.RUnlock()

	appTree, err := layoutclusterapp.ReadLayoutTreeFromCfgForApp(appName)
	if err != nil {
		oldLayers, found := s.GetCachedAppLayout(appName)
		if found {
			removeIDs := make([]int, 0, len(oldLayers))
			for _, layer := range oldLayers {
				removeIDs = append(removeIDs, layer.VID)
			}
			if err := sendSingleAppAsRemoveCommands(removeIDs); err != nil {
				return &uhmi.Response{Status: "Failed to LayoutSetApplicationLayout"}, err
			}
			s.deleteCachedAppLayout(appName)
			return &uhmi.Response{Status: "Application layout removed successfully"}, nil
		}
		return &uhmi.Response{Status: "Failed to LayoutSetApplicationLayout"}, err
	}
	return s.applyAppLayoutTree(appName, appTree, "SetApplicationLayout", "Application layout set successfully")
}

func (s *Server) DeleteApplicationLayout(ctx context.Context, req *uhmi.DeleteApplicationLayoutRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	s.layoutMutationMu.Lock()
	defer s.layoutMutationMu.Unlock()
	appName := req.GetAppName()

	if appName == "" {
		return &uhmi.Response{Status: "Failed to LayoutDeleteApplicationLayout: app_name is required"}, fmt.Errorf("app_name is required")
	}

	oldLayers, found := s.GetCachedAppLayout(appName)
	if !found {
		return &uhmi.Response{Status: "Failed to LayoutDeleteApplicationLayout: app not found in cache"}, fmt.Errorf("app %s not found in cache", appName)
	}

	removeIDs := make([]int, 0, len(oldLayers))
	for _, layer := range oldLayers {
		removeIDs = append(removeIDs, layer.VID)
	}

	if err := sendSingleAppAsRemoveCommands(removeIDs); err != nil {
		return &uhmi.Response{Status: "Failed to LayoutDeleteApplicationLayout"}, err
	}

	s.deleteCachedAppLayout(appName)
	s.logLayerHierarchy()
	return &uhmi.Response{Status: "Application layout deleted successfully"}, nil
}

func (s *Server) SetLayoutCommand(ctx context.Context, req *uhmi.SetLayoutCommandRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	layoutCommand := req.GetLayoutCommand()
	if layoutCommand == "" {
		return &uhmi.Response{Status: "Failed to LayoutSetLayoutCommand"}, fmt.Errorf("layout_command is empty")
	}

	var mJson map[string]interface{}
	if err := json.Unmarshal([]byte(layoutCommand), &mJson); err != nil {
		return failResponse("SetLayoutCommand", err)
	}
	if !isWireProtocolCommand(mJson) {
		appName, appTree, err := layoutclusterapp.LayoutTreeFromInitialLayoutJSON([]byte(layoutCommand))
		if err != nil {
			return failResponse("SetLayoutCommand", err)
		}
		return s.applyAppLayoutTree(appName, appTree, "SetLayoutCommand", "Set layout command successfully")
	}

	err := s.sendLayoutCommand(layoutCommand)
	if err != nil {
		ELog.Println(err)
		return &uhmi.Response{Status: "Failed to LayoutSetLayoutCommand"}, err
	}
	return &uhmi.Response{Status: "Set layout command successfully"}, nil
}
