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

package layoutvscreen

import (
	"encoding/json"
	"errors"
	_ "fmt"
	"sort"
	"strconv"
	"sync"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

var vScreenMutex sync.RWMutex

var applyCommandMutex sync.Mutex

var VScreen *VirtualScreen

type VirtualScreen struct {
	VScrnDef config.VScrnDef

	/* the same as vscrnDef.Size.VirtualW and vscrnDef.Size.VirtualH */
	VirtualWidth  float64
	VirtualHeight float64

	VirtualDisplays   map[int]layoutcore.VirtualDisplay
	RealDisplays      map[int]layoutcore.RealDisplay
	VdispVlayers      map[int][]layoutcore.VirtualLayer
	VdispVsafetyAreas map[int][]layoutcore.VirtualSafetyArea
}

func NewVirtualScreen(vscrnDef *config.VScrnDef) (*VirtualScreen, error) {

	vscreen := VirtualScreen{
		VScrnDef:          *vscrnDef,
		VirtualWidth:      vscrnDef.Def2D.Size.VirtualW,
		VirtualHeight:     vscrnDef.Def2D.Size.VirtualH,
		VirtualDisplays:   make(map[int]layoutcore.VirtualDisplay),
		RealDisplays:      make(map[int]layoutcore.RealDisplay),
		VdispVlayers:      make(map[int][]layoutcore.VirtualLayer),
		VdispVsafetyAreas: make(map[int][]layoutcore.VirtualSafetyArea),
	}

	for _, r := range vscrnDef.Def2D.VirtualDisplays {
		vscreen.VirtualDisplays[r.VDisplayId] = layoutcore.VirtualDisplay{
			DispName:   r.DispName,
			VDisplayId: r.VDisplayId,
			VirtualX:   r.VirtualX,
			VirtualY:   r.VirtualY,
			VirtualW:   r.VirtualW,
			VirtualH:   r.VirtualH,
		}
		vscreen.VdispVlayers[r.VDisplayId] = make([]layoutcore.VirtualLayer, 0)
	}

	for _, r := range vscrnDef.RealDisplays {
		vscreen.RealDisplays[r.VDisplayId] = layoutcore.RealDisplay{
			NodeId:     r.NodeId,
			VDisplayId: r.VDisplayId,
			PixelW:     r.PixelW,
			PixelH:     r.PixelH,
			RDisplayId: r.RDisplayId,
		}
	}

	vVdispVsafetyAreas := make([]layoutcore.VirtualSafetyArea, 0)
	for _, r := range vscrnDef.VirtualSafetyArea {
		vVdispVsafetyArea := layoutcore.VirtualSafetyArea{
			VirtualX: r.VirtualX,
			VirtualY: r.VirtualY,
			VirtualW: r.VirtualW,
			VirtualH: r.VirtualH,
		}
		vVdispVsafetyAreas = append(vVdispVsafetyAreas, vVdispVsafetyArea)
	}
	for _, r := range vscrnDef.Def2D.VirtualDisplays {
		vscreen.VdispVsafetyAreas[r.VDisplayId] = vVdispVsafetyAreas
	}

	return &vscreen, nil
}

func (vscreen *VirtualScreen) Dup() *VirtualScreen {
	vScreenMutex.RLock()
	defer vScreenMutex.RUnlock()
	copiedVDsps := make(map[int]layoutcore.VirtualDisplay)
	copiedRDsps := make(map[int]layoutcore.RealDisplay)
	copiedVDispVLayers := make(map[int][]layoutcore.VirtualLayer)
	copiedVdispVsafetyAreas := make(map[int][]layoutcore.VirtualSafetyArea)

	for vdspid, vdisplay := range vscreen.VirtualDisplays {
		copiedVDsp := vdisplay
		copiedRDsp := vscreen.RealDisplays[vdspid]
		copiedVLayers := vscreen.VdispVlayers[vdspid]
		copiedVSafetyAreas := vscreen.VdispVsafetyAreas[vdspid]

		copiedVDsps[vdspid] = copiedVDsp
		copiedRDsps[vdspid] = copiedRDsp
		copiedVDispVLayers[vdspid] = layoutcore.DupVirtualLayerSlice(copiedVLayers)
		copiedVdispVsafetyAreas[vdspid] = layoutcore.DupVirtualSafetyAreaSlice(copiedVSafetyAreas)
	}

	copiedVscreen := *vscreen
	copiedVscreen.VirtualDisplays = copiedVDsps
	copiedVscreen.RealDisplays = copiedRDsps
	copiedVscreen.VdispVlayers = copiedVDispVLayers
	copiedVscreen.VdispVsafetyAreas = copiedVdispVsafetyAreas
	return &copiedVscreen
}

func (vscrn *VirtualScreen) ApplyCommand(mJson map[string]interface{}) (*layoutcore.ApplyCommandData, error) {
	vScreenMutex.RLock()
	defer vScreenMutex.RUnlock()
	command := mJson["command"].(string)
	DLog.Println("command=", command)

	var chgIds []layoutcore.IdPair
	var err error

	switch command {
	case "initial_vscreen":
		DLog.Println("@@INITIAL_VSCREEN@@")
		chgIds, err = initVirtualScreen(vscrn, mJson)
	case "add_vsurface":
		DLog.Println("@@ADD_SURFACE@@")
		chgIds, err = addSurfaceToVLayer(vscrn, mJson)

	case "modify_vsurface":
		DLog.Println("@@MODIFY_SURFACE@@")
		chgIds, err = applySurfaceLoop(modifySurfaceInVLayer, vscrn, mJson)

	case "remove_vsurface":
		DLog.Println("@@REMOVE_SURFACE@@")
		chgIds, err = applySurfaceLoop(removeSurfaceFromVLayer, vscrn, mJson)

	case "add_vlayer":
		DLog.Println("@@ADD_LAYER@@")
		chgIds, err = addLayerToVscreen(vscrn, mJson)

	case "set_vlayer_order":
		DLog.Println("@@SET_LAYER_ORDER@@")
		chgIds, err = setVlayerOrderInVscreen(vscrn, mJson)

	case "modify_vlayer":
		DLog.Println("@@MODIFY_LAYER@@")
		chgIds, err = applyLayerLoop(modifyLayerInVscreen, vscrn, mJson)
	case "remove_vlayer":
		DLog.Println("@@REMOVE_LAYER@@")
		chgIds, err = applyLayerLoop(removeLayerFromVscreen, vscrn, mJson)
	case "resync":
		DLog.Println("@@RESYNC@@")
		chgIds = make([]layoutcore.IdPair, 0)
	default:
		chgIds = make([]layoutcore.IdPair, 0)
	}

	if err != nil {
		return nil, err
	}

	var vids []int
	for _, vdisp := range vscrn.VirtualDisplays {
		vids = make([]int, 0)
		for _, vlayer := range vscrn.VdispVlayers[vdisp.VDisplayId] {
			vids = append(vids, vlayer.VID)
		}
		DLog.Println("ApplyCommand command: ", command, " vdispId: ", vdisp.VDisplayId, " vids: ", vids)
	}

	acdata := &layoutcore.ApplyCommandData{Command: command, ChgIds: chgIds}

	return acdata, nil

}

func (vscrn *VirtualScreen) GetVlayerParams(vid int) (layoutcore.VirtualLayer, error) {
	vScreenMutex.RLock()
	defer vScreenMutex.RUnlock()
	for _, vdisp := range vscrn.VirtualDisplays {
		for _, vlayer := range vscrn.VdispVlayers[vdisp.VDisplayId] {
			if vlayer.VID == vid {
				// Copied: Vsurfaces would otherwise alias this display's
				// scene state and let callers edit it behind our back.
				return *vlayer.Dup(), nil
			}
		}
	}

	return layoutcore.VirtualLayer{}, errors.New("Cannot Find " + strconv.Itoa(vid) + " layer parameters")
}

// GetVlayerZOrderRank returns the 0-based z-order rank (slice index in
// VdispVlayers) for the layer identified by vid. 0 = bottommost layer.
// For global-coord layers that appear in multiple displays, the rank from the
// first matching display is returned (consistent with GetVlayerParams).
// Returns -1 if the layer is not found.
func (vscrn *VirtualScreen) GetVlayerZOrderRank(vid int) int {
	vScreenMutex.RLock()
	defer vScreenMutex.RUnlock()
	for _, vdisp := range vscrn.VirtualDisplays {
		for idx, vlayer := range vscrn.VdispVlayers[vdisp.VDisplayId] {
			if vlayer.VID == vid {
				return idx
			}
		}
	}
	return -1
}

// GetOrderedVlayers returns every layer currently in the scene, back to front,
// with live geometry and surfaces. Displays are walked in ascending id and
// duplicate VIDs skipped, so a layer present on only one display is still
// reported. Callers rebuilding the whole scene need this to avoid dropping
// layers they do not otherwise know about.
func (vscrn *VirtualScreen) GetOrderedVlayers() []layoutcore.VirtualLayer {
	vScreenMutex.RLock()
	defer vScreenMutex.RUnlock()

	ids := make([]int, 0, len(vscrn.VirtualDisplays))
	for id := range vscrn.VirtualDisplays {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	seen := make(map[int]bool)
	out := make([]layoutcore.VirtualLayer, 0)
	for _, id := range ids {
		for _, vlayer := range vscrn.VdispVlayers[id] {
			if seen[vlayer.VID] {
				continue
			}
			seen[vlayer.VID] = true
			out = append(out, vlayer)
		}
	}
	return out
}

func addSurfaceToVDispVLayer(vlayers *[]layoutcore.VirtualLayer, mJson map[string]interface{}) ([]layoutcore.IdPair, error) {

	chgIds := make([]layoutcore.IdPair, 0)

	surfaces, err := getSliceFromJson(mJson, "vsurface")
	if err != nil {
		return chgIds, err
	}
	layerId, err := getIntFromJson(mJson, "ParentVID")
	if err != nil {
		return chgIds, err
	}
	refId, err := getIntFromJson(mJson, "referenceVID")
	if err != nil {
		refId = -1
	}
	insert, err := getStringFromJson(mJson, "insert_order")
	if err != nil {
		return chgIds, err
	}

	layerIdx := -1
	for j, vlayer := range *vlayers {
		if vlayer.VID == layerId {
			layerIdx = j
			break
		}
	}
	if layerIdx < 0 {
		return chgIds, errors.New("Cannot Find ParentVID")
	}

	newVsurfaces := make([]layoutcore.VirtualSurface, 0)
	for _, mSurface := range surfaces {
		surfaceId, err := getIntFromJson(mSurface.(map[string]interface{}), "VID")
		if err != nil {
			return chgIds, err
		}

		for idx, vsurface := range (*vlayers)[layerIdx].Vsurfaces {
			if vsurface.VID == surfaceId {
				(*vlayers)[layerIdx].Vsurfaces = append((*vlayers)[layerIdx].Vsurfaces[:idx], (*vlayers)[layerIdx].Vsurfaces[idx+1:]...)
				break
			}
		}

		newVsurface, err := generateSurfaceFromParam(layerId, mSurface.(map[string]interface{}), (*vlayers)[layerIdx].AppName)
		if err != nil {
			return chgIds, err
		}
		newVsurfaces = append(newVsurfaces, *newVsurface)

		chgIds = append(chgIds, layoutcore.IdPair{layerId, surfaceId})
		break
	}

	for idx, vsurface := range (*vlayers)[layerIdx].Vsurfaces {
		if vsurface.VID == refId {
			if insert == layoutcore.InsertBefore {
				if idx == 0 {
					(*vlayers)[layerIdx].Vsurfaces = append(newVsurfaces, (*vlayers)[layerIdx].Vsurfaces...)
				} else {
					(*vlayers)[layerIdx].Vsurfaces = append((*vlayers)[layerIdx].Vsurfaces[:idx], append(newVsurfaces, (*vlayers)[layerIdx].Vsurfaces[idx:]...)...)
				}
			} else if insert == layoutcore.InsertAfter {
				if idx == 0 && len((*vlayers)[layerIdx].Vsurfaces) == 1 {
					(*vlayers)[layerIdx].Vsurfaces = append((*vlayers)[layerIdx].Vsurfaces, newVsurfaces...)
				} else {
					(*vlayers)[layerIdx].Vsurfaces = append((*vlayers)[layerIdx].Vsurfaces[:idx+1], append(newVsurfaces, (*vlayers)[layerIdx].Vsurfaces[idx+1:]...)...)
				}
			}
			return chgIds, nil
		}
	}

	if insert == layoutcore.InsertAppend {
		(*vlayers)[layerIdx].Vsurfaces = append((*vlayers)[layerIdx].Vsurfaces, newVsurfaces...)
	} else if insert == layoutcore.InsertPrepend {
		(*vlayers)[layerIdx].Vsurfaces = append(newVsurfaces, (*vlayers)[layerIdx].Vsurfaces...)
	}

	return chgIds, nil

}

func addSurfaceToVLayer(vscreen *VirtualScreen, mJson map[string]interface{}) ([]layoutcore.IdPair, error) {
	chgIds := make([]layoutcore.IdPair, 0)
	var err error
	for _, vdisp := range vscreen.VirtualDisplays {
		vlayers := vscreen.VdispVlayers[vdisp.VDisplayId]
		chgIds, err = addSurfaceToVDispVLayer(&vlayers, mJson)
		if err != nil {
			ELog.Println("failed to add Surface to layer: ", err)
		}
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}
	return chgIds, nil
}

func modifyVDispSurface(vlayers *[]layoutcore.VirtualLayer, layerId int, mSurface map[string]interface{}) error {

	surfaceId, err := getIntFromJson(mSurface, "VID")
	if err != nil {
		return err
	}

	for j := range *vlayers {
		vlayer := (*vlayers)[j]
		if vlayer.VID != layerId {
			continue
		}

		DLog.Println("loop", j, layerId, surfaceId)

		for idx := range vlayer.Vsurfaces {
			vsurface := vlayer.Vsurfaces[idx]

			if vsurface.VID == surfaceId {
				DLog.Println("replace surface Id=", surfaceId)
				err := modifySurfaceFromParam(&vsurface, mSurface)
				if err != nil {
					return err
				}
				vlayer.Vsurfaces[idx] = vsurface
				return nil
			}
		}
	}

	return nil
}

func modifySurfaceInVLayer(vscreen *VirtualScreen, layerId int, mSurface map[string]interface{}) error {
	for _, vdisp := range vscreen.VirtualDisplays {
		vlayers := vscreen.VdispVlayers[vdisp.VDisplayId]
		err := modifyVDispSurface(&vlayers, layerId, mSurface)
		if err != nil {
			ELog.Println("failed to modify surface params: ", err)
			return err
		}
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}
	return nil
}

func removeVDispSurface(vlayers *[]layoutcore.VirtualLayer, layerId int, mSurface map[string]interface{}) error {

	surfaceId, err := getIntFromJson(mSurface, "VID")
	if err != nil {
		return err
	}

	for j := range *vlayers {
		vlayer := (*vlayers)[j]
		if vlayer.VID != layerId {
			continue
		}

		DLog.Println("loop", j, layerId, surfaceId)

		newSurfaces := make([]layoutcore.VirtualSurface, 0)

		for idx := range vlayer.Vsurfaces {
			vsurface := vlayer.Vsurfaces[idx]

			if vsurface.VID == surfaceId {
				continue
			}

			newSurfaces = append(newSurfaces, vsurface)
		}

		(*vlayers)[j].Vsurfaces = newSurfaces
	}

	return nil
}

func removeSurfaceFromVLayer(vscreen *VirtualScreen, layerId int, mSurface map[string]interface{}) error {
	for _, vdisp := range vscreen.VirtualDisplays {
		vlayers := vscreen.VdispVlayers[vdisp.VDisplayId]
		err := removeVDispSurface(&vlayers, layerId, mSurface)
		if err != nil {
			ELog.Println("failed to remove surface: ", err)
			return err
		}
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}

	return nil
}

func applySurfaceLoop(
	dofunc func(*VirtualScreen, int, map[string]interface{}) error,
	vscreen *VirtualScreen,
	mJson map[string]interface{}) ([]layoutcore.IdPair, error) {

	layers := mJson["vlayer"].([]interface{})

	chgIds := make([]layoutcore.IdPair, 0)

	for _, mLayer := range layers {
		layerId, err := getIntFromJson(mLayer.(map[string]interface{}), "VID")
		if err != nil {
			return chgIds, err
		}
		surfaces, err := getSliceFromJson(mLayer.(map[string]interface{}), "vsurface")
		if err != nil {
			return chgIds, err
		}

		for _, mSurface := range surfaces {
			surfaceId, err := getIntFromJson(mSurface.(map[string]interface{}), "VID")
			if err != nil {
				return chgIds, err
			}

			err = dofunc(vscreen, layerId, mSurface.(map[string]interface{}))
			if err != nil {
				return chgIds, err
			}

			chgIds = append(chgIds, layoutcore.IdPair{layerId, surfaceId})
		}

	}

	return chgIds, nil
}

func addLayerToVDispScreen(vdisp layoutcore.VirtualDisplay, vlayers *[]layoutcore.VirtualLayer, mJson map[string]interface{}) ([]layoutcore.IdPair, bool, error) {

	chgIds := make([]layoutcore.IdPair, 0)
	layers := mJson["vlayer"].([]interface{})

	refId, err := getIntFromJson(mJson, "referenceVID")
	if err != nil {
		refId = -1
	}
	insert, err := getStringFromJson(mJson, "insert_order")
	if err != nil {
		return chgIds, false, err
	}

	newVlayers := make([]layoutcore.VirtualLayer, 0)
	for _, mLayer := range layers {
		layerId, err := getIntFromJson(mLayer.(map[string]interface{}), "VID")
		if err != nil {
			return chgIds, false, err
		}

		var existingVlayer *layoutcore.VirtualLayer
		vsurfaces := make([]layoutcore.VirtualSurface, 0)
		for idx, vlayer := range *vlayers {
			if vlayer.VID == layerId {
				for _, refvlayer := range *vlayers {
					if refvlayer.VID == refId {
						if refvlayer.Coord == layoutcore.COORD_GLOBAL &&
							!checkLayerInVDisplay(vdisp.VirtualX, vdisp.VirtualY, vdisp.VirtualW, vdisp.VirtualH,
								refvlayer.VdstX, refvlayer.VdstY, refvlayer.VdstW, refvlayer.VdstH) {

							DLog.Printf("skipped order change of layer %d because reference layer isn't included in VirtualDisplay %d Area: ", refId, vdisp.VDisplayId)
							// Only the stacking is meaningless here; the layer
							// parameters still have to be applied, otherwise this
							// display keeps stale geometry/visibility forever.
							updatedVlayer, uerr := generateLayerFromParam(mLayer.(map[string]interface{}), false, &vlayer)
							if uerr != nil {
								return chgIds, true, uerr
							}
							updatedVlayer.Vsurfaces = vlayer.Vsurfaces
							(*vlayers)[idx] = *updatedVlayer
							return chgIds, true, nil
						}
					}
				}
				existingVlayer = &vlayer
				vsurfaces = vlayer.Vsurfaces
				*vlayers = append((*vlayers)[:idx], (*vlayers)[idx+1:]...)
				break
			}
		}

		newVlayer, err := generateLayerFromParam(mLayer.(map[string]interface{}), false, existingVlayer)
		if err != nil {
			return chgIds, false, err
		}
		newVlayer.Vsurfaces = vsurfaces
		newVlayers = append(newVlayers, *newVlayer)

		chgIds = append(chgIds, layoutcore.IdPair{layerId, -1})
		break
	}

	for idx, vlayer := range *vlayers {
		if vlayer.VID == refId {

			if insert == layoutcore.InsertBefore {
				if idx == 0 {
					*vlayers = append(newVlayers, *vlayers...)
				} else {
					*vlayers = append((*vlayers)[:idx], append(newVlayers, (*vlayers)[idx:]...)...)
				}
			} else if insert == layoutcore.InsertAfter {
				if idx == 0 && len(*vlayers) == 1 {
					*vlayers = append(*vlayers, newVlayers...)
				} else {
					*vlayers = append((*vlayers)[:idx+1], append(newVlayers, (*vlayers)[idx+1:]...)...)
				}
			}
			return chgIds, false, nil
		}
	}

	if insert == layoutcore.InsertAppend {
		*vlayers = append(*vlayers, newVlayers...)
	} else if insert == layoutcore.InsertPrepend {
		*vlayers = append(newVlayers, *vlayers...)
	} else if len(newVlayers) > 0 {
		WLog.Printf("reference layer %d not found in VirtualDisplay %d; appending layer(s) instead of dropping", refId, vdisp.VDisplayId)
		*vlayers = append(*vlayers, newVlayers...)
	}

	return chgIds, false, nil
}

type fallbackRefInfo struct {
	order  string
	refVid int
}

func buildFallbackFromInsertedDisplay(vlayers []layoutcore.VirtualLayer, layerVid int) []fallbackRefInfo {
	for idx, vlayer := range vlayers {
		if vlayer.VID == layerVid {
			var fallbacks []fallbackRefInfo
			for back, fwd := idx-1, idx+1; back >= 0 || fwd < len(vlayers); back, fwd = back-1, fwd+1 {
				if back >= 0 {
					fallbacks = append(fallbacks, fallbackRefInfo{
						order:  layoutcore.InsertAfter,
						refVid: vlayers[back].VID,
					})
				}
				if fwd < len(vlayers) {
					fallbacks = append(fallbacks, fallbackRefInfo{
						order:  layoutcore.InsertBefore,
						refVid: vlayers[fwd].VID,
					})
				}
			}
			return fallbacks
		}
	}
	return nil
}

func sortedVDisplayIds(vscreen *VirtualScreen) []int {
	ids := make([]int, 0, len(vscreen.VirtualDisplays))
	for id := range vscreen.VirtualDisplays {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func containsVID(vlayers []layoutcore.VirtualLayer, vid int) bool {
	for _, vl := range vlayers {
		if vl.VID == vid {
			return true
		}
	}
	return false
}

func setVlayerOrderInVscreen(vscreen *VirtualScreen, mJson map[string]interface{}) ([]layoutcore.IdPair, error) {
	chgIds := make([]layoutcore.IdPair, 0)

	layers, err := getSliceFromJson(mJson, "vlayer")
	if err != nil {
		return chgIds, err
	}

	order := make([]int, 0, len(layers))
	for _, mLayer := range layers {
		vid, err := getIntFromJson(mLayer.(map[string]interface{}), "VID")
		if err != nil {
			return chgIds, err
		}
		order = append(order, vid)
	}
	if len(order) == 0 {
		return chgIds, nil
	}

	for _, dispId := range sortedVDisplayIds(vscreen) {
		cur := vscreen.VdispVlayers[dispId]
		if len(cur) == 0 {
			continue
		}

		byVID := make(map[int]layoutcore.VirtualLayer, len(cur))
		for _, vl := range cur {
			byVID[vl.VID] = vl
		}

		out := make([]layoutcore.VirtualLayer, 0, len(cur))
		placed := make(map[int]bool, len(cur))
		for _, vid := range order {
			if vl, ok := byVID[vid]; ok && !placed[vid] {
				out = append(out, vl)
				placed[vid] = true
			}
		}

		for i, vl := range cur {
			if placed[vl.VID] {
				continue
			}
			pos := 0
			for j := i - 1; j >= 0; j-- {
				if !placed[cur[j].VID] {
					continue
				}
				for k, o := range out {
					if o.VID == cur[j].VID {
						pos = k + 1
						break
					}
				}
				break
			}
			out = append(out, layoutcore.VirtualLayer{})
			copy(out[pos+1:], out[pos:])
			out[pos] = vl
			placed[vl.VID] = true
		}

		vscreen.VdispVlayers[dispId] = out
	}

	return chgIds, nil
}

func addLayerToVscreen(vscreen *VirtualScreen, mJson map[string]interface{}) ([]layoutcore.IdPair, error) {
	chgIds := make([]layoutcore.IdPair, 0)
	var err error

	refId, _ := getIntFromJson(mJson, "referenceVID")
	insert, _ := getStringFromJson(mJson, "insert_order")

	var targetVid int
	if layers, ok := mJson["vlayer"].([]interface{}); ok && len(layers) > 0 {
		if mLayer, ok := layers[0].(map[string]interface{}); ok {
			targetVid, _ = getIntFromJson(mLayer, "VID")
		}
	}

	needFallback := (insert == layoutcore.InsertBefore || insert == layoutcore.InsertAfter) && refId >= 0

	dispIds := sortedVDisplayIds(vscreen)

	var fallbackRefs []fallbackRefInfo
	handledDisps := make(map[int]bool)

	if needFallback {
		for _, dispId := range dispIds {
			vdisp := vscreen.VirtualDisplays[dispId]
			vlayers := vscreen.VdispVlayers[dispId]
			if !containsVID(vlayers, refId) {
				continue
			}

			var refOutside bool
			var ids []layoutcore.IdPair
			ids, refOutside, err = addLayerToVDispScreen(vdisp, &vlayers, mJson)
			if err != nil {
				ELog.Println("failed to add Layer to screen: ", err)
			}
			vscreen.VdispVlayers[dispId] = vlayers

			if refOutside {
				handledDisps[dispId] = true
				continue
			}
			if len(ids) > 0 {
				chgIds = ids
				handledDisps[dispId] = true

				if fallbackRefs == nil && targetVid > 0 {
					fallbackRefs = buildFallbackFromInsertedDisplay(vlayers, targetVid)
				}
			}
		}

		for _, dispId := range dispIds {
			if handledDisps[dispId] || fallbackRefs == nil {
				continue
			}
			vdisp := vscreen.VirtualDisplays[dispId]
			vlayers := vscreen.VdispVlayers[dispId]

			inserted := false
			for _, fb := range fallbackRefs {
				if !containsVID(vlayers, fb.refVid) {
					continue
				}
				mJson["referenceVID"] = float64(fb.refVid)
				mJson["insert_order"] = fb.order
				var ids []layoutcore.IdPair
				ids, _, err = addLayerToVDispScreen(vdisp, &vlayers, mJson)
				if err != nil {
					ELog.Println("failed to add Layer to screen: ", err)
				}
				if len(ids) > 0 {
					chgIds = ids
					inserted = true
					break
				}
			}
			if !inserted {
				DLog.Printf("addLayerToVscreen: no fallback ref found for VDisplay %d, order unchanged", dispId)
			}

			vscreen.VdispVlayers[dispId] = vlayers
		}
		mJson["referenceVID"] = float64(refId)
		mJson["insert_order"] = insert
	} else {
		for _, dispId := range dispIds {
			vdisp := vscreen.VirtualDisplays[dispId]
			vlayers := vscreen.VdispVlayers[dispId]
			var ids []layoutcore.IdPair
			ids, _, err = addLayerToVDispScreen(vdisp, &vlayers, mJson)
			if err != nil {
				ELog.Println("failed to add Layer to screen: ", err)
			}
			if len(ids) > 0 {
				chgIds = ids
			}
			vscreen.VdispVlayers[dispId] = vlayers
		}
	}

	return chgIds, nil
}

func modifyVDispVLayer(vlayers *[]layoutcore.VirtualLayer, mLayer map[string]interface{}) error {

	layerId, err := getIntFromJson(mLayer, "VID")
	if err != nil {
		return err
	}

	for idx := range *vlayers {
		vlayer := (*vlayers)[idx]
		if vlayer.VID == layerId {
			err := modifyLayerFromParam(&vlayer, mLayer)
			if err != nil {
				return err
			}
			(*vlayers)[idx] = vlayer
			return nil
		}
	}

	return nil
}

func modifyLayerInAllDisplays(vscreen *VirtualScreen, mLayer map[string]interface{}) error {
	for _, vdisp := range vscreen.VirtualDisplays {
		vlayers := vscreen.VdispVlayers[vdisp.VDisplayId]
		err := modifyVDispVLayer(&vlayers, mLayer)
		if err != nil {
			ELog.Println("failed to modify layer params: ", err)
			return err
		}
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}
	return nil
}

func modifyLayerInVscreen(vscreen *VirtualScreen, mLayer map[string]interface{}) error {
	layerId, err := getIntFromJson(mLayer, "VID")
	if err != nil {
		return err
	}

	cmdCoord, coordErr := getCoordFromJson(mLayer, "coord")
	if coordErr != nil {
		return modifyLayerInAllDisplays(vscreen, mLayer)
	}

	var existing *layoutcore.VirtualLayer
	for _, dispId := range sortedVDisplayIds(vscreen) {
		for idx := range vscreen.VdispVlayers[dispId] {
			if vscreen.VdispVlayers[dispId][idx].VID == layerId {
				existing = vscreen.VdispVlayers[dispId][idx].Dup()
				break
			}
		}
		if existing != nil {
			break
		}
	}
	if existing == nil {
		return nil
	}

	keep := make(map[int]bool)
	switch cmdCoord {
	case layoutcore.COORD_GLOBAL:
		for dispId := range vscreen.VirtualDisplays {
			keep[dispId] = true
		}
	case layoutcore.COORD_VDISPLAY:
		targetId := existing.VDisplayId
		if vid, err := getIntFromJsonDef(mLayer, "vdisplay_id", -1); err == nil && vid >= 0 {
			targetId = vid
		}
		if _, ok := vscreen.VirtualDisplays[targetId]; !ok {
			WLog.Printf("modify_vlayer VID=%d: unknown vdisplay_id %d; layer left unchanged", layerId, targetId)
			return nil
		}
		keep[targetId] = true
	}

	for _, dispId := range sortedVDisplayIds(vscreen) {
		vlayers := vscreen.VdispVlayers[dispId]
		if keep[dispId] {
			if containsVID(vlayers, layerId) {
				if err := modifyVDispVLayer(&vlayers, mLayer); err != nil {
					ELog.Println("failed to modify layer params: ", err)
					return err
				}
			} else {
				restored := existing.Dup()
				if err := modifyLayerFromParam(restored, mLayer); err != nil {
					return err
				}
				vlayers = append(vlayers, *restored)
			}
		} else {
			filtered := make([]layoutcore.VirtualLayer, 0, len(vlayers))
			for _, vl := range vlayers {
				if vl.VID == layerId {
					continue
				}
				filtered = append(filtered, vl)
			}
			vlayers = filtered
		}
		vscreen.VdispVlayers[dispId] = vlayers
	}
	return nil
}

func removeVDispLayer(vlayers *[]layoutcore.VirtualLayer, mLayer map[string]interface{}) error {

	layerId, err := getIntFromJson(mLayer, "VID")
	if err != nil {
		return err
	}

	newVlayers := make([]layoutcore.VirtualLayer, 0)
	for idx := range *vlayers {
		vlayer := (*vlayers)[idx]
		if vlayer.VID == layerId {
			continue
		}
		newVlayers = append(newVlayers, vlayer)
	}

	*vlayers = newVlayers

	return nil
}

func removeLayerFromVscreen(vscreen *VirtualScreen, mLayer map[string]interface{}) error {
	var err error
	for _, vdisp := range vscreen.VirtualDisplays {
		vlayers := vscreen.VdispVlayers[vdisp.VDisplayId]
		err = removeVDispLayer(&vlayers, mLayer)
		if err != nil {
			ELog.Println("failed to remove layer: ", err)
			return err
		}
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}
	return nil
}

func applyLayerLoop(
	dofunc func(*VirtualScreen, map[string]interface{}) error,
	vscreen *VirtualScreen,
	mJson map[string]interface{}) ([]layoutcore.IdPair, error) {

	layers := mJson["vlayer"].([]interface{})

	chgIds := make([]layoutcore.IdPair, 0)

	for _, mLayer := range layers {

		layerId, err := getIntFromJson(mLayer.(map[string]interface{}), "VID")
		if err != nil {
			return chgIds, err
		}

		err = dofunc(vscreen, mLayer.(map[string]interface{}))
		if err != nil {
			return chgIds, err
		}

		chgIds = append(chgIds, layoutcore.IdPair{layerId, -1})
	}

	return chgIds, nil
}

func initVirtualScreen(vscreen *VirtualScreen, mJson map[string]interface{}) ([]layoutcore.IdPair, error) {

	vlayers := make([]layoutcore.VirtualLayer, 0)

	layers, err := getSliceFromJson(mJson, "vlayer")
	if err != nil {
		ELog.Println("err in fillVscreenFromParam")
		return make([]layoutcore.IdPair, 0), err
	}

	for _, mLayer := range layers {
		var existingVlayer *layoutcore.VirtualLayer
		newVlayer, err := generateLayerFromParam(mLayer.(map[string]interface{}), true, existingVlayer)
		if err != nil {
			ELog.Println("err in fillVscreenFromParam")
			return make([]layoutcore.IdPair, 0), err
		}
		vlayers = append(vlayers, *newVlayer)
	}

	for _, vdisp := range vscreen.VirtualDisplays {
		vscreen.VdispVlayers[vdisp.VDisplayId] = vlayers
	}

	return make([]layoutcore.IdPair, 0), nil
}

func checkLayerInVDisplay(
	vdisp_vx float64,
	vdisp_vy float64,
	vdisp_vw float64,
	vdisp_vh float64,
	vlayer_vdx float64,
	vlayer_vdy float64,
	vlayer_vdw float64,
	vlayer_vdh float64) bool {
	if vdisp_vw <= 0 || vdisp_vh <= 0 || vlayer_vdw <= 0 || vlayer_vdh <= 0 {
		return false
	}

	return layoutcore.LayersOverlap(
		vdisp_vx, vdisp_vy, vdisp_vw, vdisp_vh,
		vlayer_vdx, vlayer_vdy, vlayer_vdw, vlayer_vdh,
	)

}

func ApplyAndGenCommand(command string, nodeId int) (string, error) {
	var applyCommand map[string]interface{}
	if err := json.Unmarshal([]byte(command), &applyCommand); err != nil {
		ELog.Printf("Unmarshal json command error: %s\n", err)
		return "", err
	}

	applyCommandMutex.Lock()
	defer applyCommandMutex.Unlock()

	vscrnCopy := VScreen.Dup()

	acdata, err := vscrnCopy.ApplyCommand(applyCommand)
	if err != nil {
		ELog.Printf("ApplyCommand error: %s\n", err)
		return "", err
	}

	vs2rdConv, err := NewVscreen2RdisplayConverter(vscrnCopy, nodeId)
	if err != nil {
		ELog.Printf("Failed to create converter: %s\n", err)
		return "", err
	}

	var vsconv GeometoryConverter = vs2rdConv
	vsconv.DoConvert()

	acdata.NPScreens, err = vsconv.GetNodePixelScreens()
	if err != nil {
		ELog.Printf("GetNodePixelScreens error: %s\n", err)
		return "", err
	}

	jsonBytes, err := json.Marshal(acdata)
	if err != nil {
		ELog.Printf("Marshal ApplyCommandData error: %s\n", err)
		return "", err
	}

	jsonCommand := string(jsonBytes)

	vScreenMutex.Lock()
	VScreen = vscrnCopy
	vScreenMutex.Unlock()

	return jsonCommand, nil
}
