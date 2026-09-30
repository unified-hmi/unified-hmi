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

package layoutcommgen

import (
	"encoding/json"
	"unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

func convVirtualSurface2UPIVsurface(vsurf *layoutcore.VirtualSurface, appName string) *UPIVsurface {
	usurf := new(UPIVsurface)

	usurf.AppName = appName

	usurf.VID = vsurf.VID

	usurf.PixelW = vsurf.PixelW
	usurf.PixelH = vsurf.PixelH

	usurf.PsrcX = vsurf.PsrcX
	usurf.PsrcY = vsurf.PsrcY
	usurf.PsrcW = vsurf.PsrcW
	usurf.PsrcH = vsurf.PsrcH

	usurf.VdstX = vsurf.VdstX
	usurf.VdstY = vsurf.VdstY
	usurf.VdstW = vsurf.VdstW
	usurf.VdstH = vsurf.VdstH

	usurf.Visibility = vsurf.Visibility
	usurf.WlSurfaceId = vsurf.WlSurfaceId
	return usurf
}

func convVirtualLayer2UPIVlayer(vlayer *layoutcore.VirtualLayer) *UPIVlayer {
	ulayer := new(UPIVlayer)

	ulayer.AppName = vlayer.AppName

	ulayer.VID = vlayer.VID
	ulayer.Coord = vlayer.Coord
	ulayer.VdisplayId = vlayer.VDisplayId

	ulayer.VirtualW = vlayer.VirtualW
	ulayer.VirtualH = vlayer.VirtualH

	ulayer.VsrcX = vlayer.VsrcX
	ulayer.VsrcY = vlayer.VsrcY
	ulayer.VsrcW = vlayer.VsrcW
	ulayer.VsrcH = vlayer.VsrcH

	ulayer.VdstX = vlayer.VdstX
	ulayer.VdstY = vlayer.VdstY
	ulayer.VdstW = vlayer.VdstW
	ulayer.VdstH = vlayer.VdstH

	ulayer.Visibility = vlayer.Visibility

	ulayer.Surface = make([]UPIVsurface, 0)

	for _, vsurf := range vlayer.Vsurfaces {
		copiedVsurf := vsurf
		usurf := convVirtualSurface2UPIVsurface(&copiedVsurf, ulayer.AppName)
		ulayer.Surface = append(ulayer.Surface, *usurf)
	}

	return ulayer
}

// GenerateCommSetVlayerOrder builds a command that carries nothing but the
// intended z-order. Geometry is untouched, and because the command is not
// initial_vscreen the worker keeps diffing against its previous state, so
// animation frames still produce modify_layer.
func GenerateCommSetVlayerOrder(vids []int) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "set_vlayer_order"

	for _, vid := range vids {
		uscreen.Layer = append(uscreen.Layer, UPIVlayer{VID: vid})
	}

	jsonBytes, err := json.Marshal(uscreen)
	if err != nil {
		ELog.Println("JSON Marshal error: ", err)
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, nil
}

func GenerateCommInitialVscreen(ctree *layoutcore.LayoutTree) (string, error) {

	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "initial_vscreen"

	for _, layer := range ctree.Vlayers {
		ulayer := convVirtualLayer2UPIVlayer(&layer)
		uscreen.Layer = append(uscreen.Layer, *ulayer)
	}

	jsonBytes, err := json.Marshal(uscreen)
	if err != nil {
		ELog.Println("JSON Marshal error: ", err)
		return msg, err
	}

	msg = string(jsonBytes)

	return msg, nil
}

// GenerateCommResync builds a minimal layout command whose sole purpose is to
// nudge every worker's plugin pipeline into running its reconnect check and
// (if applicable) replaying the cached initial_layout to a compositor that
// just came back online. It carries no scene diff.
func GenerateCommResync() (string, error) {
	uscreen := &UPIVscreen{Command: "resync"}
	jsonBytes, err := json.Marshal(uscreen)
	if err != nil {
		ELog.Println("JSON Marshal error: ", err)
		return "", err
	}
	return string(jsonBytes), nil
}

func GenerateCommGetVlayer(vid int) (string, error) {

	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "get_vlayer"

	ulayers := make([]UPIVlayer, 0)

	ulayer := UPIVlayer{
		VID: vid,
	}
	ulayers = append(ulayers, ulayer)

	uscreen.Layer = ulayers

	jsonBytes, err := json.Marshal(uscreen)
	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, nil
}

func GenerateCommModifyVlayer(vlayer layoutcore.VirtualLayer) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "modify_vlayer"

	ulayer := convVirtualLayer2UPIVlayer(&vlayer)
	uscreen.Layer = append(uscreen.Layer, *ulayer)

	jsonBytes, err := json.Marshal(uscreen)
	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, nil
}

func GenerateCommAddVlayer(vlayer layoutcore.VirtualLayer, order string, refvid int) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "add_vlayer"
	uscreen.InsertOrder = order
	uscreen.ReferenceVID = refvid

	ulayer := convVirtualLayer2UPIVlayer(&vlayer)
	uscreen.Layer = append(uscreen.Layer, *ulayer)

	jsonBytes, err := json.Marshal(uscreen)

	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, err
}

func GenerateCommRemoveVlayer(vlayer layoutcore.VirtualLayer) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "remove_vlayer"

	ulayer := convVirtualLayer2UPIVlayer(&vlayer)
	uscreen.Layer = append(uscreen.Layer, *ulayer)

	jsonBytes, err := json.Marshal(uscreen)

	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, err
}

func GenerateCommModifyVsurface(vlayer layoutcore.VirtualLayer) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "modify_vsurface"

	ulayer := convVirtualLayer2UPIVlayer(&vlayer)
	uscreen.Layer = append(uscreen.Layer, *ulayer)

	jsonBytes, err := json.Marshal(uscreen)

	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, err
}

func GenerateCommAddVsurface(vsurface layoutcore.VirtualSurface, order string, refvid int, parentvid int) (string, error) {
	var msg string
	uscreen := new(UPIVscreen)
	uscreen.Command = "add_vsurface"
	uscreen.InsertOrder = order
	uscreen.ReferenceVID = refvid
	uscreen.ParentVID = parentvid

	usurface := convVirtualSurface2UPIVsurface(&vsurface, "")
	uscreen.Surface = append(uscreen.Surface, *usurface)

	jsonBytes, err := json.Marshal(uscreen)

	if err != nil {
		return msg, err
	}

	msg = string(jsonBytes)
	return msg, err
}
