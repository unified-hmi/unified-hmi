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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"unified-hmi/internal/layout/backend"
	"unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

const UHMI_RVGPU_LAYOUT_SOCK string = "uhmi-rvgpu_layout_sock"
const VERSION string = "0.0.0"
const OPACITY float64 = 1.0

type rvgpuLayoutJson struct {
	Id             int     `json:"id"`
	RvgpuSurfaceID string  `json:"rvgpu_surface_id"`
	WlSurfaceId    int     `json:"wl_surface_id,omitempty"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	SrcX           int     `json:"src_x"`
	SrcY           int     `json:"src_y"`
	SrcW           int     `json:"src_w"`
	SrcH           int     `json:"src_h"`
	DstX           int     `json:"dst_x"`
	DstY           int     `json:"dst_y"`
	DstW           int     `json:"dst_w"`
	DstH           int     `json:"dst_h"`
	Opacity        float64 `json:"opacity"`
	Visibility     int     `json:"visibility"`
}

type safetyAreaJson struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type InitialLayoutProtocol struct {
	Version      string            `json:"version"`
	Command      string            `json:"command"`
	RvgpuLayouts []rvgpuLayoutJson `json:"surfaces"`
	SafetyAreas  []safetyAreaJson  `json:"safety_areas"`
}

type AddSurfaceProtocol struct {
	Version      string            `json:"version"`
	Command      string            `json:"command"`
	InsertOrder  string            `json:"insert_order"`
	ReferenceId  int               `json:"referenceID"`
	RvgpuLayouts []rvgpuLayoutJson `json:"surfaces"`
}

type ModifySurfaceProtocol struct {
	Version      string            `json:"version"`
	Command      string            `json:"command"`
	RvgpuLayouts []rvgpuLayoutJson `json:"surfaces"`
}

type RemoveSurfaceProtocol struct {
	Version      string            `json:"version"`
	Command      string            `json:"command"`
	RvgpuLayouts []rvgpuLayoutJson `json:"surfaces"`
}

type Key struct {
	RDisplayId int
	LayerID    int
}

var layerSurfacesMap = make(map[Key][]layoutcore.PixelSurface)
var mapMutex = sync.RWMutex{}

func addOrUpdateLayerSurfaces(rDisplayID int, layerID int, surfaces []layoutcore.PixelSurface) {
	mapMutex.Lock()
	defer mapMutex.Unlock()
	layerSurfacesMap[Key{rDisplayID, layerID}] = surfaces
}

func getLayerSurfaces(rDisplayID int, layerID int) ([]layoutcore.PixelSurface, bool) {
	mapMutex.RLock()
	defer mapMutex.RUnlock()
	surfaces, exists := layerSurfacesMap[Key{rDisplayID, layerID}]
	return surfaces, exists
}

func deleteLayer(rDisplayID int, layerID int) {
	mapMutex.Lock()
	defer mapMutex.Unlock()

	delete(layerSurfacesMap, Key{rDisplayID, layerID})
}

func deleteSurfaceFromLayer(rDisplayID int, layerID int, surfaceID int) {
	mapMutex.Lock()
	defer mapMutex.Unlock()

	surfaces, exists := layerSurfacesMap[Key{rDisplayID, layerID}]
	if !exists {
		return
	}

	for i, surface := range surfaces {
		if surface.VID == surfaceID {
			layerSurfacesMap[Key{rDisplayID, layerID}] = append(surfaces[:i], surfaces[i+1:]...)
			break
		}
	}
}

func genRvgpuSurfaceID(lVID int, sVID int) string {

	combined := fmt.Sprintf("%d%d", lVID, sVID)

	hash := sha256.New()
	hash.Write([]byte(combined))
	hashBytes := hash.Sum(nil)

	hashString := hex.EncodeToString(hashBytes)

	return hashString[:8]
}

func genRvgpuLayoutParams(player layoutcore.PixelLayer, psurfaces []layoutcore.PixelSurface) []rvgpuLayoutJson {
	var rvgpuLayouts []rvgpuLayoutJson
	for _, psurf := range psurfaces {
		viewSrcX, viewSrcY, viewSrcW, viewSrcH, viewDstX, viewDstY, viewDstW, viewDstH := calcSrcViewArea(player, psurf)

		// The rvgpu protocol has no layer object, so a hidden layer has to
		// be expressed by hiding each of its surfaces.
		visibility := psurf.Visibility
		if player.Visibility == 0 {
			visibility = 0
		}

		rvgpuLayout := rvgpuLayoutJson{
			Id:             player.VID,
			RvgpuSurfaceID: psurf.AppName,
			WlSurfaceId:    psurf.WlSurfaceId,
			Width:          int(psurf.PixelW),
			Height:         int(psurf.PixelH),
			SrcX:           int(viewSrcX),
			SrcY:           int(viewSrcY),
			SrcW:           int(viewSrcW),
			SrcH:           int(viewSrcH),
			DstX:           int(viewDstX),
			DstY:           int(viewDstY),
			DstW:           int(viewDstW),
			DstH:           int(viewDstH),
			Opacity:        OPACITY,
			Visibility:     visibility,
		}
		rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout)
	}
	return rvgpuLayouts
}

func calcSrcViewArea(player layoutcore.PixelLayer, psurface layoutcore.PixelSurface) (float64, float64, float64, float64, float64, float64, float64, float64) {
	srcX, srcW, dstX, dstW := calcSurfaceAxisView(
		player.PsrcX, player.PsrcW,
		player.PdstX, player.PdstW,
		psurface.PsrcX, psurface.PsrcW,
		psurface.PdstX, psurface.PdstW,
	)
	srcY, srcH, dstY, dstH := calcSurfaceAxisView(
		player.PsrcY, player.PsrcH,
		player.PdstY, player.PdstH,
		psurface.PsrcY, psurface.PsrcH,
		psurface.PdstY, psurface.PdstH,
	)

	return layoutcore.RoundTo5(srcX), layoutcore.RoundTo5(srcY),
		layoutcore.RoundTo5(srcW), layoutcore.RoundTo5(srcH),
		layoutcore.RoundTo5(dstX), layoutcore.RoundTo5(dstY),
		layoutcore.RoundTo5(dstW), layoutcore.RoundTo5(dstH)
}

func calcSurfaceAxisView(
	layerSrcStart, layerSrcSize float64,
	layerDstStart, layerDstSize float64,
	surfaceSrcStart, surfaceSrcSize float64,
	surfaceDstStart, surfaceDstSize float64,
) (surfaceViewStart, surfaceViewSize, layerViewStart, layerViewSize float64) {
	if layerSrcSize <= 0 || layerDstSize <= 0 ||
		surfaceSrcSize < 0 || surfaceDstSize <= 0 {
		return 0, 0, 0, 0
	}

	intersectionStart := math.Max(layerSrcStart, surfaceDstStart)
	intersectionEnd := math.Min(
		layerSrcStart+layerSrcSize,
		surfaceDstStart+surfaceDstSize,
	)
	if intersectionEnd <= intersectionStart {
		return 0, 0, 0, 0
	}

	visibleSize := intersectionEnd - intersectionStart
	surfaceScale := surfaceSrcSize / surfaceDstSize
	layerScale := layerDstSize / layerSrcSize

	surfaceViewStart = surfaceSrcStart +
		(intersectionStart-surfaceDstStart)*surfaceScale
	surfaceViewSize = visibleSize * surfaceScale
	layerViewStart = layerDstStart +
		(intersectionStart-layerSrcStart)*layerScale
	layerViewSize = visibleSize * layerScale

	return surfaceViewStart, surfaceViewSize, layerViewStart, layerViewSize
}

func genInitialLayoutProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {
	var rvgpuLayouts []rvgpuLayoutJson
	var safetyareas []safetyAreaJson

	for _, rdcomm := range req.RDComms {

		if rdcomm.Rdisplay.RDisplayId == rId {

			for _, player := range rdcomm.Players {

				rvgpuLayout := genRvgpuLayoutParams(player, player.Psurfaces)
				addOrUpdateLayerSurfaces(rId, player.VID, player.Psurfaces)

				rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout...)
			}
			for _, r := range rdcomm.SafetyAreas {
				rvgpuLayout := safetyAreaJson{
					X:      int(r.PixelX),
					Y:      int(r.PixelY),
					Width:  int(r.PixelW),
					Height: int(r.PixelH),
				}
				safetyareas = append(safetyareas, rvgpuLayout)
			}
		}
	}

	rvgpuProto := InitialLayoutProtocol{
		Version:      VERSION,
		Command:      "initial_layout",
		RvgpuLayouts: rvgpuLayouts,
		SafetyAreas:  safetyareas,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}

func genAddLayerProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {

	var rvgpuLayouts []rvgpuLayoutJson
	var insertOrder string
	var referenceId int
	for _, rdcomm := range req.RDComms {
		insertOrder = rdcomm.InsertOrder
		referenceId = rdcomm.ReferenceId
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {

				psurfaces, exist := getLayerSurfaces(rId, player.VID)
				if exist {
					rvgpuLayout := genRvgpuLayoutParams(player, psurfaces)
					rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout...)
				} else {
					continue
				}
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := AddSurfaceProtocol{
		Version:      VERSION,
		Command:      "add_surface",
		InsertOrder:  insertOrder,
		ReferenceId:  referenceId,
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}

func genModifyLayerProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {

	var rvgpuLayouts []rvgpuLayoutJson
	for _, rdcomm := range req.RDComms {
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {
				psurfaces, exist := getLayerSurfaces(rId, player.VID)
				if exist {
					rvgpuLayout := genRvgpuLayoutParams(player, psurfaces)
					rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout...)
				} else {
					continue
				}
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := ModifySurfaceProtocol{
		Version:      VERSION,
		Command:      "modify_surface",
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)
	return msg, nil
}

func genRemoveLayerProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {
	var rvgpuLayouts []rvgpuLayoutJson

	for _, rdcomm := range req.RDComms {
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {
				psurfaces, exist := getLayerSurfaces(rId, player.VID)
				if exist {
					for _, psurf := range psurfaces {
						rvgpuLayout := rvgpuLayoutJson{
							Id:             player.VID,
							RvgpuSurfaceID: psurf.AppName,
						}
						deleteSurfaceFromLayer(rId, player.VID, psurf.VID)
						rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout)
					}
					deleteLayer(rId, player.VID)
				} else {
					continue
				}
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := RemoveSurfaceProtocol{
		Version:      VERSION,
		Command:      "remove_surface",
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}

func genAddSurfaceProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {

	var rvgpuLayouts []rvgpuLayoutJson
	var insertOrder string
	var referenceId int
	for _, rdcomm := range req.RDComms {
		insertOrder = rdcomm.InsertOrder
		referenceId = rdcomm.ReferenceId
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {
				rvgpuLayout := genRvgpuLayoutParams(player, player.Psurfaces)
				addOrUpdateLayerSurfaces(rId, player.VID, player.Psurfaces)
				rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout...)
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := AddSurfaceProtocol{
		Version:      VERSION,
		Command:      req.Command,
		InsertOrder:  insertOrder,
		ReferenceId:  referenceId,
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}

func genModifySurfaceProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {

	var rvgpuLayouts []rvgpuLayoutJson
	for _, rdcomm := range req.RDComms {
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {
				rvgpuLayout := genRvgpuLayoutParams(player, player.Psurfaces)
				addOrUpdateLayerSurfaces(rId, player.VID, player.Psurfaces)
				rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout...)
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := ModifySurfaceProtocol{
		Version:      VERSION,
		Command:      req.Command,
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}

func genRemoveSurfaceProtocolJson(req layoutbackend.LocalCommandReq, rId int) (string, error) {

	var rvgpuLayouts []rvgpuLayoutJson
	for _, rdcomm := range req.RDComms {
		if rdcomm.Rdisplay.RDisplayId == rId {
			for _, player := range rdcomm.Players {
				for _, psurf := range player.Psurfaces {
					rvgpuLayout := rvgpuLayoutJson{
						Id:             player.VID,
						RvgpuSurfaceID: psurf.AppName,
					}
					deleteSurfaceFromLayer(rId, player.VID, psurf.VID)
					rvgpuLayouts = append(rvgpuLayouts, rvgpuLayout)
				}
			}
		}
	}

	if len(rvgpuLayouts) == 0 {
		return "", nil
	}

	rvgpuProto := RemoveSurfaceProtocol{
		Version:      VERSION,
		Command:      req.Command,
		RvgpuLayouts: rvgpuLayouts,
	}

	jsonBytes, err := json.Marshal(rvgpuProto)
	if err != nil {
		ELog.Println("JSON Marshal error:", err)
	}

	msg := string(jsonBytes)

	return msg, nil
}
