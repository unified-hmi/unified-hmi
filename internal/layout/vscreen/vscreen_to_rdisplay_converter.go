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
	"errors"
	"math"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/core"
)

type GeometoryConverter interface {
	DoConvert() error
	GetNodePixelScreens() (*layoutcore.NodePixelScreens, error)
}

type WorkV2R struct {
	vdisplay     layoutcore.VirtualDisplay
	rdisplay     layoutcore.RealDisplay
	vlayers      []layoutcore.VirtualLayer
	vsafetyareas []layoutcore.VirtualSafetyArea

	players      []layoutcore.PixelLayer
	psafetyareas []layoutcore.PixelSafetyArea
}

type Vscreen2RdisplayConverter struct {
	nodeId     int
	workV2RMap map[int]WorkV2R
}

func NewVscreen2RdisplayConverter(
	vscrn *VirtualScreen,
	nodeId int) (*Vscreen2RdisplayConverter, error) {

	wdisplay, err := generateWorkV2R(vscrn, nodeId, &vscrn.VScrnDef)
	if err != nil {
		return nil, errors.New("NewVscreen2RdisplayConverter error")
	}

	vs2rd := &Vscreen2RdisplayConverter{
		nodeId:     nodeId,
		workV2RMap: wdisplay,
	}

	return vs2rd, nil
}

func (vs2rd *Vscreen2RdisplayConverter) DoConvert() error {

	convVDisplay2RDisplayCoordinate(vs2rd.workV2RMap)

	return nil
}

func convVSurface2PSurface(vsurf *layoutcore.VirtualSurface) *layoutcore.PixelSurface {

	psurf := layoutcore.NewEmptyPixelSurface()

	psurf.AppName = vsurf.AppName

	psurf.ParentVID = vsurf.ParentVID
	psurf.VID = vsurf.VID

	psurf.PixelW = vsurf.PixelW
	psurf.PixelH = vsurf.PixelH

	psurf.PsrcX = vsurf.PsrcX
	psurf.PsrcY = vsurf.PsrcY
	psurf.PsrcW = vsurf.PsrcW
	psurf.PsrcH = vsurf.PsrcH

	psurf.PdstX = vsurf.VdstX
	psurf.PdstY = vsurf.VdstY
	psurf.PdstW = vsurf.VdstW
	psurf.PdstH = vsurf.VdstH

	// An unset visibility means visible, as everywhere else in the stack.
	psurf.Visibility = 1
	if vsurf.Visibility != nil {
		psurf.Visibility = *vsurf.Visibility
	}

	psurf.WlSurfaceId = vsurf.WlSurfaceId

	return psurf
}

func convVLayer2PLayer(vlayer *layoutcore.VirtualLayer) *layoutcore.PixelLayer {

	player := layoutcore.NewEmptyPixelLayer()

	player.AppName = vlayer.AppName

	player.VID = vlayer.VID

	player.PixelW = vlayer.VirtualW
	player.PixelH = vlayer.VirtualH

	player.PsrcX = vlayer.VsrcX
	player.PsrcY = vlayer.VsrcY
	player.PsrcW = vlayer.VsrcW
	player.PsrcH = vlayer.VsrcH

	player.PdstX = vlayer.VdstX
	player.PdstY = vlayer.VdstY
	player.PdstW = vlayer.VdstW
	player.PdstH = vlayer.VdstH

	// An unset visibility means visible, as everywhere else in the stack.
	player.Visibility = 1
	if vlayer.Visibility != nil {
		player.Visibility = *vlayer.Visibility
	}

	player.Psurfaces = make([]layoutcore.PixelSurface, 0)

	for _, sVsurf := range vlayer.Vsurfaces {
		copiedPSurf := convVSurface2PSurface(&sVsurf)
		player.Psurfaces = append(player.Psurfaces, *copiedPSurf)
	}

	return player
}

func convVLayers2PLayers(sVlayers []layoutcore.VirtualLayer) (dPlayers []layoutcore.PixelLayer) {
	if sVlayers == nil {
		return nil
	}

	dPlayers = make([]layoutcore.PixelLayer, 0)

	for _, sVlayer := range sVlayers {
		dPlayer := convVLayer2PLayer(&sVlayer)
		dPlayers = append(dPlayers, *dPlayer)
	}

	return dPlayers
}

func convVSafetyArea2PSafetyArea(vSafetyArea *layoutcore.VirtualSafetyArea) *layoutcore.PixelSafetyArea {

	pSafetyArea := layoutcore.NewEmptyPixelSafetyArea()

	pSafetyArea.PixelX = vSafetyArea.VirtualX
	pSafetyArea.PixelY = vSafetyArea.VirtualY
	pSafetyArea.PixelW = vSafetyArea.VirtualW
	pSafetyArea.PixelH = vSafetyArea.VirtualH

	return pSafetyArea
}

func convVSafetyAreas2PSafetyAreas(sVSafetyAreas []layoutcore.VirtualSafetyArea) (dPSafetyAreas []layoutcore.PixelSafetyArea) {
	if sVSafetyAreas == nil {
		return nil
	}

	dPSafetyAreas = make([]layoutcore.PixelSafetyArea, 0)

	for _, sVSafetyArea := range sVSafetyAreas {
		dPSafetyArea := convVSafetyArea2PSafetyArea(&sVSafetyArea)
		dPSafetyAreas = append(dPSafetyAreas, *dPSafetyArea)
	}

	return dPSafetyAreas
}

func isNeedForWorkV2R(vlayer *layoutcore.VirtualLayer, arg interface{}) bool {
	vDisplayId := arg.(int)

	if vlayer.Coord == layoutcore.COORD_GLOBAL {
		return true
	}

	if vlayer.Coord == layoutcore.COORD_VDISPLAY &&
		vlayer.VDisplayId == vDisplayId {
		return true
	}

	return false
}

func generateWorkV2R(
	vscreen *VirtualScreen,
	nodeId int,
	vscrnDef *config.VScrnDef) (map[int]WorkV2R, error) {

	workV2RMap := make(map[int]WorkV2R)

	for vdspid, vdisplay := range vscreen.VirtualDisplays {

		isMe := vscrnDef.IsVDisplayInNode(nodeId, vdspid)
		if !isMe {
			continue
		}

		rdisplay := vscreen.RealDisplays[vdspid]

		wvdisp := WorkV2R{
			vdisplay:     *vdisplay.Dup(),
			rdisplay:     *rdisplay.Dup(),
			vlayers:      layoutcore.DupVirtualLayerSliceIfNeed(vscreen.VdispVlayers[vdspid], isNeedForWorkV2R, vdspid),
			vsafetyareas: layoutcore.DupVirtualSafetyAreaSlice(vscreen.VdispVsafetyAreas[vdspid]),
		}

		workV2RMap[vdspid] = wvdisp
	}

	return workV2RMap, nil
}

func convGlobalToVDisplayCoordinateSub(
	vdisp_vx float64,
	vdisp_vw float64,
	vlayer_vdx float64,
	vlayer_vdw float64,
	vlayer_vsx float64,
	vlayer_vsw float64) (float64, float64, float64, float64) {

	localIntersectionStart, visibleDstSize := intersectAxis(
		vdisp_vx, vdisp_vw, vlayer_vdx, vlayer_vdw,
	)
	if visibleDstSize == 0 {
		return 0, 0, 0, 0
	}

	intersectionStart := vdisp_vx + localIntersectionStart
	srcScale := vlayer_vsw / vlayer_vdw
	visibleSrcStart := vlayer_vsx + (intersectionStart-vlayer_vdx)*srcScale
	visibleSrcSize := visibleDstSize * srcScale

	return intersectionStart - vdisp_vx, visibleDstSize, visibleSrcStart, visibleSrcSize

}

func intersectAxis(displayStart, displaySize, areaStart, areaSize float64) (float64, float64) {
	if displaySize <= 0 || areaSize <= 0 {
		return 0, 0
	}

	intersectionStart := math.Max(displayStart, areaStart)
	intersectionEnd := math.Min(
		displayStart+displaySize,
		areaStart+areaSize,
	)
	if intersectionEnd <= intersectionStart {
		return 0, 0
	}

	return intersectionStart - displayStart, intersectionEnd - intersectionStart
}

func convGlobalToVDisplayCoordinate(sVlayer *layoutcore.VirtualLayer,
	vdisp *layoutcore.VirtualDisplay) *layoutcore.VirtualLayer {

	vdisp_vx := vdisp.VirtualX
	vdisp_vy := vdisp.VirtualY
	vdisp_vw := vdisp.VirtualW
	vdisp_vh := vdisp.VirtualH

	newVlayer := sVlayer.Dup()

	vlayer_vdx := sVlayer.VdstX
	vlayer_vdy := sVlayer.VdstY
	vlayer_vdw := sVlayer.VdstW
	vlayer_vdh := sVlayer.VdstH

	vlayer_vsx := sVlayer.VsrcX
	vlayer_vsy := sVlayer.VsrcY
	vlayer_vsw := sVlayer.VsrcW
	vlayer_vsh := sVlayer.VsrcH

	nvlayer_vdx, nvlayer_vdw, nvlayer_vsx, nvlayer_vsw :=
		convGlobalToVDisplayCoordinateSub(vdisp_vx, vdisp_vw, vlayer_vdx, vlayer_vdw, vlayer_vsx, vlayer_vsw)
	nvlayer_vdy, nvlayer_vdh, nvlayer_vsy, nvlayer_vsh :=
		convGlobalToVDisplayCoordinateSub(vdisp_vy, vdisp_vh, vlayer_vdy, vlayer_vdh, vlayer_vsy, vlayer_vsh)

	newVlayer.VdstX = nvlayer_vdx
	newVlayer.VdstW = nvlayer_vdw
	newVlayer.VsrcX = nvlayer_vsx
	newVlayer.VsrcW = nvlayer_vsw

	newVlayer.VdstY = nvlayer_vdy
	newVlayer.VsrcY = nvlayer_vsy
	newVlayer.VdstH = nvlayer_vdh
	newVlayer.VsrcH = nvlayer_vsh

	return newVlayer
}

func convToVDisplayCoordinate(sVlayers []layoutcore.VirtualLayer,
	vdisp *layoutcore.VirtualDisplay) []layoutcore.VirtualLayer {

	dVlayers := make([]layoutcore.VirtualLayer, 0)

	for _, vlayer := range sVlayers {

		var newVlayer layoutcore.VirtualLayer
		if vlayer.Coord == layoutcore.COORD_VDISPLAY {
			vdisplayLocal := *vdisp.Dup()
			vdisplayLocal.VirtualX = 0
			vdisplayLocal.VirtualY = 0
			newVlayer = *convGlobalToVDisplayCoordinate(&vlayer, &vdisplayLocal)
		} else {
			newVlayer = *convGlobalToVDisplayCoordinate(&vlayer, vdisp)
		}

		dVlayers = append(dVlayers, newVlayer)
	}

	return dVlayers
}

func sAreaConvGlobalToVDisplayCoordinateSub(
	vdisp_vx float64,
	vdisp_vw float64,
	vlayer_vdx float64,
	vlayer_vdw float64) (float64, float64) {
	return intersectAxis(vdisp_vx, vdisp_vw, vlayer_vdx, vlayer_vdw)
}

func sAreaConvGlobalToVDisplayCoordinate(sVsafetyArea *layoutcore.VirtualSafetyArea,
	vdisp *layoutcore.VirtualDisplay) *layoutcore.VirtualSafetyArea {

	vdisp_vx := vdisp.VirtualX
	vdisp_vy := vdisp.VirtualY
	vdisp_vw := vdisp.VirtualW
	vdisp_vh := vdisp.VirtualH

	newVsafetyarea := sVsafetyArea.Dup()

	vsafetyarea_vdx := sVsafetyArea.VirtualX
	vsafetyarea_vdy := sVsafetyArea.VirtualY
	vsafetyarea_vdw := sVsafetyArea.VirtualW
	vsafetyarea_vdh := sVsafetyArea.VirtualH

	nvlayer_vdx, nvlayer_vdw :=
		sAreaConvGlobalToVDisplayCoordinateSub(vdisp_vx, vdisp_vw, vsafetyarea_vdx, vsafetyarea_vdw)

	nvlayer_vdy, nvlayer_vdh :=
		sAreaConvGlobalToVDisplayCoordinateSub(vdisp_vy, vdisp_vh, vsafetyarea_vdy, vsafetyarea_vdh)

	newVsafetyarea.VirtualX = nvlayer_vdx
	newVsafetyarea.VirtualY = nvlayer_vdy
	newVsafetyarea.VirtualW = nvlayer_vdw
	newVsafetyarea.VirtualH = nvlayer_vdh

	return newVsafetyarea
}

func sAreaConvToVDisplayCoordinate(sVSafetyAreas []layoutcore.VirtualSafetyArea,
	vdisp *layoutcore.VirtualDisplay) []layoutcore.VirtualSafetyArea {

	dVSafetyAreas := make([]layoutcore.VirtualSafetyArea, 0)

	for _, vSafetyArea := range sVSafetyAreas {

		var newVSafetyArea layoutcore.VirtualSafetyArea
		newVSafetyArea = *sAreaConvGlobalToVDisplayCoordinate(&vSafetyArea, vdisp)

		dVSafetyAreas = append(dVSafetyAreas, newVSafetyArea)
	}

	return dVSafetyAreas
}

func sAreaConvToRDisplayCoordinate(sVSafetyAreas []layoutcore.VirtualSafetyArea,
	vdisp *layoutcore.VirtualDisplay, rdisp *layoutcore.RealDisplay) []layoutcore.VirtualSafetyArea {

	dVSafetyAreas := make([]layoutcore.VirtualSafetyArea, 0)

	vdisp_vw := vdisp.VirtualW
	vdisp_vh := vdisp.VirtualH

	rdisp_pixw := float64(rdisp.PixelW)
	rdisp_pixh := float64(rdisp.PixelH)

	for _, vSafetyArea := range sVSafetyAreas {

		newVSafetyArea := vSafetyArea.Dup()

		vSafetyArea_vdx := vSafetyArea.VirtualX
		vSafetyArea_vdy := vSafetyArea.VirtualY
		vSafetyArea_vdw := vSafetyArea.VirtualW
		vSafetyArea_vdh := vSafetyArea.VirtualH

		newVSafetyArea.VirtualX = layoutcore.RoundTo5(vSafetyArea_vdx * rdisp_pixw / vdisp_vw)
		newVSafetyArea.VirtualW = layoutcore.RoundTo5(vSafetyArea_vdw * rdisp_pixw / vdisp_vw)
		newVSafetyArea.VirtualY = layoutcore.RoundTo5(vSafetyArea_vdy * rdisp_pixh / vdisp_vh)
		newVSafetyArea.VirtualH = layoutcore.RoundTo5(vSafetyArea_vdh * rdisp_pixh / vdisp_vh)

		dVSafetyAreas = append(dVSafetyAreas, *newVSafetyArea)
	}

	return dVSafetyAreas
}

func convToRDisplayCoordinate(sVlayers []layoutcore.VirtualLayer,
	vdisp *layoutcore.VirtualDisplay, rdisp *layoutcore.RealDisplay) []layoutcore.VirtualLayer {

	dVlayers := make([]layoutcore.VirtualLayer, 0)

	vdisp_vw := vdisp.VirtualW
	vdisp_vh := vdisp.VirtualH

	rdisp_pixw := float64(rdisp.PixelW)
	rdisp_pixh := float64(rdisp.PixelH)

	for _, vlayer := range sVlayers {

		newVlayer := vlayer.Dup()

		vlayer_vdx := vlayer.VdstX
		vlayer_vdy := vlayer.VdstY
		vlayer_vdw := vlayer.VdstW
		vlayer_vdh := vlayer.VdstH

		newVlayer.VdstX = layoutcore.RoundTo5(vlayer_vdx * rdisp_pixw / vdisp_vw)
		newVlayer.VdstW = layoutcore.RoundTo5(vlayer_vdw * rdisp_pixw / vdisp_vw)
		newVlayer.VdstY = layoutcore.RoundTo5(vlayer_vdy * rdisp_pixh / vdisp_vh)
		newVlayer.VdstH = layoutcore.RoundTo5(vlayer_vdh * rdisp_pixh / vdisp_vh)

		newVlayer.VsrcX = layoutcore.RoundTo5(newVlayer.VsrcX)
		newVlayer.VsrcY = layoutcore.RoundTo5(newVlayer.VsrcY)
		newVlayer.VsrcW = layoutcore.RoundTo5(newVlayer.VsrcW)
		newVlayer.VsrcH = layoutcore.RoundTo5(newVlayer.VsrcH)

		dVlayers = append(dVlayers, *newVlayer)
	}

	return dVlayers
}

func convVDisplay2RDisplayCoordinate(workV2RMap map[int]WorkV2R) {

	for key, workV2R := range workV2RMap {
		tmpVlayers := convToVDisplayCoordinate(workV2R.vlayers, &workV2R.vdisplay)
		vlayers := convToRDisplayCoordinate(tmpVlayers, &workV2R.vdisplay, &workV2R.rdisplay)
		workV2R.players = convVLayers2PLayers(vlayers)

		tmpVSAreas := sAreaConvToVDisplayCoordinate(workV2R.vsafetyareas, &workV2R.vdisplay)
		vSAreas := sAreaConvToRDisplayCoordinate(tmpVSAreas, &workV2R.vdisplay, &workV2R.rdisplay)
		workV2R.psafetyareas = convVSafetyAreas2PSafetyAreas(vSAreas)

		workV2RMap[key] = workV2R
	}
}

func (vs2rd *Vscreen2RdisplayConverter) GetNodePixelScreens() (*layoutcore.NodePixelScreens, error) {

	pscrns := make([]layoutcore.PixelScreen, 0)

	for _, workV2R := range vs2rd.workV2RMap {

		pscrn, err := layoutcore.NewPixelScreen(&workV2R.rdisplay, workV2R.players, workV2R.psafetyareas)
		if err != nil {
			return nil, err
		}
		pscrns = append(pscrns, *pscrn)
	}

	spscrns, err := layoutcore.NewNodePixelScreens(vs2rd.nodeId, pscrns)
	if err != nil {
		return nil, err
	}

	return spscrns, nil
}
