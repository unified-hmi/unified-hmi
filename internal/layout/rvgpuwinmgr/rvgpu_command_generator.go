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
	"errors"
	"reflect"
	"unified-hmi/internal/layout/backend"
	"unified-hmi/internal/layout/core"
)

type position struct {
	insertOrder string
	referenceId int
}

type workRvgpu struct {
	rdisplay layoutcore.RealDisplay

	players      []layoutcore.PixelLayer
	psafetyareas []layoutcore.PixelSafetyArea
}

type RvgpuCommandGenerator struct {
	workRvgpuMap    map[int]workRvgpu
	oldworkRvgpuMap map[int]workRvgpu
}

type RvgpuPlugin struct{}

func (plugin RvgpuPlugin) GenerateLocalCommandReq(acdata *layoutcore.ApplyCommandData, sps *layoutcore.NodePixelScreens) ([]*layoutbackend.LocalCommandReq, error) {
	setLatestState(sps)

	ltqs := []*layoutbackend.LocalCommandReq{}

	wRvgpuMap, err := generateWorkRvgpu(acdata.NPScreens.Dup())
	if err != nil {
		return ltqs, errors.New("generateWorkRvgpu error")
	}

	oldwRvgpuMap := make(map[int]workRvgpu)
	if acdata.Command != "initial_vscreen" {
		oldwRvgpuMap, err = generateWorkRvgpu(sps.Dup())
		if err != nil {
			return ltqs, errors.New("generateWorkRvgpu error")
		}
	}

	ret, err := pickupInitialVScreen(wRvgpuMap, oldwRvgpuMap)
	if err == nil && ret != nil {
		ltqs = append(ltqs, ret)
	}

	rets, err := pickupAddLayer(wRvgpuMap, oldwRvgpuMap)
	if err == nil && rets != nil {
		ltqs = append(ltqs, rets...)
	}
	ret, err = pickupRemoveLayer(wRvgpuMap, oldwRvgpuMap)
	if err == nil && ret != nil {
		ltqs = append(ltqs, ret)
	}
	ret, err = pickupModifyLayer(wRvgpuMap, oldwRvgpuMap)
	if err == nil && ret != nil {
		ltqs = append(ltqs, ret)
	}

	rets, err = pickupAddSurface(wRvgpuMap, oldwRvgpuMap)
	if err == nil && rets != nil {
		ltqs = append(ltqs, rets...)
	}
	ret, err = pickupRemoveSurface(wRvgpuMap, oldwRvgpuMap)
	if err == nil && ret != nil {
		ltqs = append(ltqs, ret)
	}
	ret, err = pickupModifySurface(wRvgpuMap, oldwRvgpuMap)
	if err == nil && ret != nil {
		ltqs = append(ltqs, ret)
	}

	ltqs = append(ltqs, &layoutbackend.LocalCommandReq{Command: "local_comm"})

	return ltqs, nil
}

func generateWorkRvgpu(
	spscrns *layoutcore.NodePixelScreens) (map[int]workRvgpu, error) {

	workRvgpuMap := make(map[int]workRvgpu)

	for _, pscrn := range spscrns.Pscreens {
		wvdisp := workRvgpu{
			rdisplay:     *pscrn.Rdisplay.Dup(),
			players:      layoutcore.DupPixelLayerSlice(pscrn.Players),
			psafetyareas: layoutcore.DupPixelSafetyAreaSlice(pscrn.PsafetyAreas),
		}
		workRvgpuMap[wvdisp.rdisplay.RDisplayId] = wvdisp
	}

	err := isValidWorkRvgpuMap(workRvgpuMap)

	return workRvgpuMap, err
}

func isValidWorkRvgpuMap(wRvgpuMap map[int]workRvgpu) error {

	rdisplayMap := make(map[int]bool)
	for _, workRvgpu := range wRvgpuMap {
		if rdisplayMap[workRvgpu.rdisplay.RDisplayId] {
			return errors.New("PixelScreens has duplicate RDisplayId")
		}
		rdisplayMap[workRvgpu.rdisplay.RDisplayId] = true

		playerMap := make(map[int]bool)
		for _, player := range workRvgpu.players {
			if playerMap[player.VID] {
				return errors.New("RealDisplay has duplicate PixelLayer VID")
			}
			playerMap[player.VID] = true

			psurfaceMap := make(map[int]bool)
			for _, psurface := range player.Psurfaces {
				if psurfaceMap[psurface.VID] {
					return errors.New("PixelLayer has duplicate PixelSurface VID")
				}
				psurfaceMap[psurface.VID] = true
			}
		}
	}

	return nil
}

func getVID(item interface{}) int {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return int(v.FieldByName("VID").Int())
}

func isChanged(targetItem interface{}, baseItems interface{}, targetIdx int) (bool, bool) {

	add := true
	modify := false

	if reflect.ValueOf(baseItems).Len() > targetIdx {
		baseItem := reflect.ValueOf(baseItems).Index(targetIdx).Interface()
		if getVID(targetItem) == getVID(baseItem) {
			add = false
			if !reflect.DeepEqual(targetItem, baseItem) {
				modify = true
			}
		}
	}

	return add, modify
}

func generateLocalCommand(lcomm string,
	workRvgpuMap map[int]workRvgpu,
	pickupPlayersMap map[int][]layoutcore.PixelLayer) (*layoutbackend.LocalCommandReq, error) {

	dcomms := make([]layoutbackend.RdisplayCommandData, 0)

	for key, workRvgpu := range workRvgpuMap {
		pickupPlayers := pickupPlayersMap[key]
		if len(pickupPlayers) == 0 {
			continue
		}
		dcomm, err := layoutbackend.NewRdisplayCommandData(&workRvgpu.rdisplay, pickupPlayers)
		if err != nil {
			return nil, err
		}

		dcomms = append(dcomms, *dcomm)
	}

	ltq, err := layoutbackend.NewEmptyLocalCommandReq()
	if err != nil {
		return nil, err
	}
	ltq.Command = lcomm
	ltq.RDComms = dcomms

	return ltq, nil
}

func generateLocalCommandWithPosition(lcomm string,
	wRvgpu workRvgpu, pos position,
	player layoutcore.PixelLayer) (*layoutbackend.LocalCommandReq, error) {

	dcomms := make([]layoutbackend.RdisplayCommandData, 0)

	dcomm, err := layoutbackend.NewRdisplayCommandDataWithPosition(&wRvgpu.rdisplay, pos.insertOrder, pos.referenceId, player)
	if err != nil {
		return nil, err
	}
	dcomms = append(dcomms, *dcomm)

	ltq, err := layoutbackend.NewEmptyLocalCommandReq()
	if err != nil {
		return nil, err
	}

	ltq.Command = lcomm
	ltq.RDComms = dcomms

	return ltq, nil
}

func generateLocalCommandWithSafetyAreas(lcomm string,
	workRvgpuMap map[int]workRvgpu,
	pickupPlayersMap map[int][]layoutcore.PixelLayer,
	pickupPSafetyAreasMap map[int][]layoutcore.PixelSafetyArea) (*layoutbackend.LocalCommandReq, error) {

	dcomms := make([]layoutbackend.RdisplayCommandData, 0)

	for key, workRvgpu := range workRvgpuMap {
		pickupPlayers := pickupPlayersMap[key]
		pickupSafetyAreas := pickupPSafetyAreasMap[key]
		if len(pickupPlayers) == 0 {
			continue
		}
		dcomm, err := layoutbackend.NewRdisplayCommandDataWithSafetyArea(&workRvgpu.rdisplay, pickupPlayers, pickupSafetyAreas)
		if err != nil {
			return nil, err
		}

		dcomms = append(dcomms, *dcomm)
	}

	ltq, err := layoutbackend.NewEmptyLocalCommandReq()
	if err != nil {
		return nil, err
	}
	ltq.Command = lcomm
	ltq.RDComms = dcomms

	return ltq, nil
}

func pickupInitialVScreen(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) (*layoutbackend.LocalCommandReq, error) {

	lcomm := ""
	pickupPlayersMap := make(map[int][]layoutcore.PixelLayer, len(wRvgpuMap))
	pickupPsafetyAreasMap := make(map[int][]layoutcore.PixelSafetyArea, len(wRvgpuMap))

	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		pickupPlayers := pickupPlayersMap[key]
		pickupPsafetyareas := pickupPsafetyAreasMap[key]

		if len(oldwRvgpu.players) == 0 && len(wRvgpu.players) != 0 {
			lcomm = "initial_vscreen"
			pickupPlayers = layoutcore.DupPixelLayerSlice(wRvgpu.players)
			pickupPsafetyareas = layoutcore.DupPixelSafetyAreaSlice(wRvgpu.psafetyareas)

			oldwRvgpuMap[key] = wRvgpu
			pickupPlayersMap[key] = pickupPlayers
			pickupPsafetyAreasMap[key] = pickupPsafetyareas
		}
	}

	if lcomm == "" {
		return nil, nil
	}
	ltq, err := generateLocalCommandWithSafetyAreas(lcomm, wRvgpuMap, pickupPlayersMap, pickupPsafetyAreasMap)
	if err != nil {
		return nil, err
	}

	return ltq, nil
}

func pickupAddLayer(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) ([]*layoutbackend.LocalCommandReq, error) {

	ltqs := []*layoutbackend.LocalCommandReq{}

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		oldPlayers := oldwRvgpu.players

		for lidx, player := range wRvgpu.players {
			isPlAdd, _ := isChanged(*player.DupWithoutSurface(),
				layoutcore.DupPixelLayerSliceWithoutSurface(oldPlayers), lidx)
			if isPlAdd == false {
				continue
			}

			lcomm = "add_layer"
			pickupPlayer := *player.DupWithoutSurface()

			var pos position
			if lidx == 0 {
				pos.insertOrder = layoutcore.InsertPrepend
			} else {
				pos.insertOrder = layoutcore.InsertAfter
				pos.referenceId = wRvgpu.players[lidx-1].VID
			}

			tmpRemovedPlayer := layoutcore.PixelLayer{}
			for lidx, tmpOldPlayer := range oldPlayers {
				if tmpOldPlayer.VID == player.VID {
					oldPlayers = append(oldPlayers[:lidx], oldPlayers[lidx+1:]...)
					tmpRemovedPlayer = tmpOldPlayer
					break
				}
			}
			updatedPlayer := *player.DupWithoutSurface()
			if len(tmpRemovedPlayer.Psurfaces) > 0 {
				updatedPlayer.Psurfaces = tmpRemovedPlayer.Psurfaces
			}
			if len(oldPlayers) >= lidx+1 {
				oldPlayers = append(oldPlayers[:lidx+1], oldPlayers[lidx:]...)
				oldPlayers[lidx] = updatedPlayer
			} else {
				oldPlayers = append(oldPlayers, updatedPlayer)
			}

			ltq, err := generateLocalCommandWithPosition(lcomm, wRvgpu, pos, pickupPlayer)
			if err != nil {
				return nil, err
			}
			ltqs = append(ltqs, ltq)
		}
		oldwRvgpu.players = oldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}

	return ltqs, nil
}

func pickupRemoveLayer(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) (*layoutbackend.LocalCommandReq, error) {

	pickupPlayersMap := make(map[int][]layoutcore.PixelLayer, len(wRvgpuMap))

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		oldPlayers := oldwRvgpu.players

		pickupPlayers := pickupPlayersMap[key]

		tmpOldPlayers := make([]layoutcore.PixelLayer, 0)
		tmpOldPlayers = layoutcore.DupPixelLayerSlice(oldPlayers)
		for _, oldPlayer := range oldPlayers {
			var found bool
			for _, player := range wRvgpu.players {
				if oldPlayer.VID == player.VID {
					found = true
				}
			}
			if !found {

				lcomm = "remove_layer"
				pickupPlayers = append(pickupPlayers, *oldPlayer.DupWithoutSurface())

				for lidx, tmpOldPlayer := range tmpOldPlayers {
					if oldPlayer.VID == tmpOldPlayer.VID {
						tmpOldPlayers = append(tmpOldPlayers[:lidx], tmpOldPlayers[lidx+1:]...)
						break
					}
				}
			}
		}
		pickupPlayersMap[key] = pickupPlayers

		oldwRvgpu.players = tmpOldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}
	ltq, err := generateLocalCommand(lcomm, wRvgpuMap, pickupPlayersMap)
	if err != nil {
		return nil, err
	}

	return ltq, nil
}

func pickupModifyLayer(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) (*layoutbackend.LocalCommandReq, error) {

	pickupPlayersMap := make(map[int][]layoutcore.PixelLayer, len(wRvgpuMap))

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		oldPlayers := oldwRvgpu.players

		pickupPlayers := pickupPlayersMap[key]

		for lidx, player := range wRvgpu.players {
			_, isPlMod := isChanged(*player.DupWithoutSurface(),
				layoutcore.DupPixelLayerSliceWithoutSurface(oldPlayers), lidx)
			if isPlMod == false {
				continue
			}

			lcomm = "modify_layer"
			pickupPlayers = append(pickupPlayers, *player.DupWithoutSurface())

			updatedPlayer := *player.DupWithoutSurface()
			if len(oldPlayers[lidx].Psurfaces) > 0 {
				updatedPlayer.Psurfaces = oldPlayers[lidx].Psurfaces
			}
			oldPlayers[lidx] = updatedPlayer
		}
		pickupPlayersMap[key] = pickupPlayers

		oldwRvgpu.players = oldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}
	ltq, err := generateLocalCommand(lcomm, wRvgpuMap, pickupPlayersMap)
	if err != nil {
		return nil, err
	}

	return ltq, nil
}

func pickupAddSurface(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) ([]*layoutbackend.LocalCommandReq, error) {

	ltqs := []*layoutbackend.LocalCommandReq{}

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		oldPlayers := oldwRvgpu.players

		for lidx, player := range wRvgpu.players {
			oldPsurfaces := oldPlayers[lidx].Psurfaces

			for sidx, psurface := range player.Psurfaces {
				isPsAdd, _ := isChanged(psurface, oldPsurfaces, sidx)
				if isPsAdd == false {
					continue
				}

				lcomm = "add_surface"
				pickupPlayer := *player.DupWithoutSurface()
				pickupPlayer.Psurfaces = append(pickupPlayer.Psurfaces, *psurface.Dup())

				var pos position
				if sidx == 0 {
					if lidx == 0 {
						pos.insertOrder = layoutcore.InsertPrepend
					} else {
						pos.insertOrder = layoutcore.InsertAfter
						pos.referenceId = wRvgpu.players[lidx-1].VID
					}
				} else {
					pos.insertOrder = layoutcore.InsertAfter
					pos.referenceId = player.Psurfaces[sidx-1].VID
				}

				for sidx, oldPsurface := range oldPsurfaces {
					if oldPsurface.VID == psurface.VID {
						oldPsurfaces = append(oldPsurfaces[:sidx], oldPsurfaces[sidx+1:]...)
					}
				}
				if len(oldPsurfaces) >= sidx+1 {
					oldPsurfaces = append(oldPsurfaces[:sidx+1], oldPsurfaces[sidx:]...)
					oldPsurfaces[sidx] = *psurface.Dup()
				} else {
					oldPsurfaces = append(oldPsurfaces, *psurface.Dup())
				}

				ltq, err := generateLocalCommandWithPosition(lcomm, wRvgpu, pos, pickupPlayer)
				if err != nil {
					return nil, err
				}
				ltqs = append(ltqs, ltq)
			}
			oldPlayers[lidx].Psurfaces = oldPsurfaces
		}
		oldwRvgpu.players = oldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}

	return ltqs, nil
}

func pickupRemoveSurface(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) (*layoutbackend.LocalCommandReq, error) {

	pickupPlayersMap := make(map[int][]layoutcore.PixelLayer, len(wRvgpuMap))

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		pickupPlayers := pickupPlayersMap[key]

		oldPlayers := oldwRvgpu.players

		for lidx, player := range wRvgpu.players {
			oldPsurfaces := oldPlayers[lidx].Psurfaces

			pickupPSurfaces := make([]layoutcore.PixelSurface, 0)

			tmpOldPsurfaces := make([]layoutcore.PixelSurface, 0)
			tmpOldPsurfaces = layoutcore.DupPixelSurfaceSlice(oldPsurfaces)
			for _, oldPsurface := range oldPsurfaces {
				var found bool
				for _, psurface := range player.Psurfaces {
					if oldPsurface.VID == psurface.VID {
						found = true
					}
				}
				if !found {

					lcomm = "remove_surface"
					pickupPSurfaces = append(pickupPSurfaces, *oldPsurface.Dup())

					for sidx, tmpOldPsurface := range tmpOldPsurfaces {
						if oldPsurface.VID == tmpOldPsurface.VID {
							tmpOldPsurfaces = append(tmpOldPsurfaces[:sidx], tmpOldPsurfaces[sidx+1:]...)
							break
						}
					}
				}
			}

			if len(pickupPSurfaces) != 0 {
				pickupPlayer := player.DupWithoutSurface()
				pickupPlayer.Psurfaces = pickupPSurfaces

				pickupPlayers = append(pickupPlayers, *pickupPlayer)
				oldPlayers[lidx].Psurfaces = tmpOldPsurfaces
			}
		}
		pickupPlayersMap[key] = pickupPlayers

		oldwRvgpu.players = oldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}
	ltq, err := generateLocalCommand(lcomm, wRvgpuMap, pickupPlayersMap)
	if err != nil {
		return nil, err
	}

	return ltq, nil
}

func pickupModifySurface(
	wRvgpuMap map[int]workRvgpu,
	oldwRvgpuMap map[int]workRvgpu) (*layoutbackend.LocalCommandReq, error) {

	pickupPlayersMap := make(map[int][]layoutcore.PixelLayer, len(wRvgpuMap))

	lcomm := ""
	for key, wRvgpu := range wRvgpuMap {
		oldwRvgpu := oldwRvgpuMap[key]
		oldPlayers := oldwRvgpu.players

		pickupPlayers := pickupPlayersMap[key]

		for lidx, player := range wRvgpu.players {
			oldPsurfaces := oldPlayers[lidx].Psurfaces

			pickupPSurfaces := make([]layoutcore.PixelSurface, 0)

			for sidx, psurface := range player.Psurfaces {
				_, isPsMod := isChanged(psurface, oldPsurfaces, sidx)
				if isPsMod == true {

					lcomm = "modify_surface"
					pickupPSurfaces = append(pickupPSurfaces, *psurface.Dup())

					oldPsurfaces[sidx] = psurface
				}
			}

			if len(pickupPSurfaces) != 0 {
				pickupPlayer := player.DupWithoutSurface()
				pickupPlayer.Psurfaces = pickupPSurfaces

				pickupPlayers = append(pickupPlayers, *pickupPlayer)
				oldPlayers[lidx].Psurfaces = oldPsurfaces
			}
		}
		pickupPlayersMap[key] = pickupPlayers

		oldwRvgpu.players = oldPlayers
		oldwRvgpuMap[key] = oldwRvgpu
	}

	if lcomm == "" {
		return nil, nil
	}
	ltq, err := generateLocalCommand(lcomm, wRvgpuMap, pickupPlayersMap)
	if err != nil {
		return nil, err
	}

	return ltq, nil
}
