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
	_ "encoding/json"
	"errors"
	"unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

func getStringFromJson(mJson map[string]interface{}, key string) (string, error) {

	tval := mJson[key]
	if tval == nil {
		return "", errors.New("Error in getStringFromJson")
	}
	val := tval.(string)

	return val, nil
}

func getIntFromJson(mJson map[string]interface{}, key string) (int, error) {

	tval := mJson[key]
	if tval == nil {
		return 0, errors.New("Error in getIntFromJson")
	}
	val := int(tval.(float64))

	return val, nil
}

func getIntFromJsonDef(mJson map[string]interface{}, key string, defval int) (int, error) {

	tval := mJson[key]
	if tval == nil {
		return defval, nil
	}
	val := int(tval.(float64))

	return val, nil
}

func getFloat64FromJson(mJson map[string]interface{}, key string) (float64, error) {

	tval := mJson[key]
	if tval == nil {
		return 0, errors.New("Error in getFloat64FromJson")
	}
	val := tval.(float64)

	return val, nil
}

func getSliceFromJson(mJson map[string]interface{}, key string) ([]interface{}, error) {

	tval := mJson[key]
	if tval == nil {
		return make([]interface{}, 0), errors.New("Error in getSliceFromJson")
	}
	val := tval.([]interface{})

	return val, nil
}

func getCoordFromJson(mJson map[string]interface{}, key string) (string, error) {

	tval := mJson[key]
	if tval == nil {
		return "", errors.New("getCoordFromJson")
	}

	if tval == layoutcore.COORD_GLOBAL {
		return layoutcore.COORD_GLOBAL, nil
	} else if tval == layoutcore.COORD_VDISPLAY {
		return layoutcore.COORD_VDISPLAY, nil
	}

	return "", errors.New("getCoordFromJson")
}

func getCoordFromJsonDef(mJson map[string]interface{}, key string, defval string) (string, error) {

	tval := mJson[key]
	if tval == nil {
		return defval, nil
	}

	if tval == layoutcore.COORD_GLOBAL {
		return layoutcore.COORD_GLOBAL, nil
	} else if tval == layoutcore.COORD_VDISPLAY {
		return layoutcore.COORD_VDISPLAY, nil
	}

	return "", errors.New("getCoordFromJsonDef")
}

func generateSurfaceFromParam(layerId int, mSurface map[string]interface{}, appli_name string) (*layoutcore.VirtualSurface, error) {

	surfaceId, err := getIntFromJson(mSurface, "VID")
	if err != nil {
		return nil, err
	}

	pixelW, err := getFloat64FromJson(mSurface, "pixel_w")
	if err != nil {
		return nil, err
	}

	pixelH, err := getFloat64FromJson(mSurface, "pixel_h")
	if err != nil {
		return nil, err
	}

	psrcX, err := getFloat64FromJson(mSurface, "psrc_x")
	if err != nil {
		return nil, err
	}

	psrcY, err := getFloat64FromJson(mSurface, "psrc_y")
	if err != nil {
		return nil, err
	}

	psrcW, err := getFloat64FromJson(mSurface, "psrc_w")
	if err != nil {
		return nil, err
	}

	psrcH, err := getFloat64FromJson(mSurface, "psrc_h")
	if err != nil {
		return nil, err
	}

	vdstX, err := getFloat64FromJson(mSurface, "vdst_x")
	if err != nil {
		return nil, err
	}

	vdstY, err := getFloat64FromJson(mSurface, "vdst_y")
	if err != nil {
		return nil, err
	}

	vdstW, err := getFloat64FromJson(mSurface, "vdst_w")
	if err != nil {
		return nil, err
	}

	vdstH, err := getFloat64FromJson(mSurface, "vdst_h")
	if err != nil {
		return nil, err
	}

	if pixelW < 0 || pixelH < 0 {
		return nil, errors.New("pixel_w and pixel_h should not have negative values")
	}

	if psrcX < 0 || psrcY < 0 || psrcW < 0 || psrcH < 0 {
		return nil, errors.New("psrc regions should not have negative values")
	}

	if vdstX < 0 || vdstY < 0 || vdstW < 0 || vdstH < 0 {
		return nil, errors.New("vdst regions should not have negative values")
	}

	visibility, err := getIntFromJsonDef(mSurface, "visibility", 1)
	if err != nil {
		return nil, err
	}

	wlSurfaceId, err := getIntFromJsonDef(mSurface, "wl_surface_id", 0)
	if err != nil {
		return nil, err
	}

	vsurface := layoutcore.VirtualSurface{
		AppName:     appli_name,
		ParentVID:   layerId,
		VID:         surfaceId,
		PixelW:      pixelW,
		PixelH:      pixelH,
		PsrcX:       psrcX,
		PsrcY:       psrcY,
		PsrcW:       psrcW,
		PsrcH:       psrcH,
		VdstX:       vdstX,
		VdstY:       vdstY,
		VdstW:       vdstW,
		VdstH:       vdstH,
		Visibility:  &visibility,
		WlSurfaceId: wlSurfaceId,
	}

	return &vsurface, nil
}

func generateLayerFromParam(mLayer map[string]interface{}, genSurfaces bool, existingVlayer *layoutcore.VirtualLayer) (*layoutcore.VirtualLayer, error) {

	appli_name, err := getStringFromJson(mLayer, "appli_name")
	if err != nil {
		ELog.Println("error in generateLayerFromParam")
		return nil, err
	}

	layerId, err := getIntFromJson(mLayer, "VID")
	if err != nil {
		if existingVlayer == nil {
			return nil, err
		} else {
			layerId = existingVlayer.VID
		}
	}

	var defCoord string
	if existingVlayer != nil {
		defCoord = existingVlayer.Coord
	} else {
		defCoord = layoutcore.COORD_GLOBAL
	}
	coord, err := getCoordFromJsonDef(mLayer, "coord", defCoord)
	if err != nil {
		ELog.Println("error in generateLayerFromParam")
		return nil, err
	}

	vdisplayId := -1
	if coord == layoutcore.COORD_VDISPLAY {
		defVdisplayId := -1
		if existingVlayer != nil {
			defVdisplayId = existingVlayer.VDisplayId
		}
		vdisplayId, err = getIntFromJsonDef(mLayer, "vdisplay_id", defVdisplayId)
		if err != nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		}
	}

	virtualW, err := getFloat64FromJson(mLayer, "virtual_w")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			virtualW = existingVlayer.VirtualW
		}
	}

	virtualH, err := getFloat64FromJson(mLayer, "virtual_h")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			virtualH = existingVlayer.VirtualH
		}
	}

	vsrcX, err := getFloat64FromJson(mLayer, "vsrc_x")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vsrcX = existingVlayer.VsrcX
		}
	}

	vsrcY, err := getFloat64FromJson(mLayer, "vsrc_y")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vsrcY = existingVlayer.VsrcY
		}
	}

	vsrcW, err := getFloat64FromJson(mLayer, "vsrc_w")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vsrcW = existingVlayer.VsrcW
		}
	}

	vsrcH, err := getFloat64FromJson(mLayer, "vsrc_h")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vsrcH = existingVlayer.VsrcH
		}
	}

	vdstX, err := getFloat64FromJson(mLayer, "vdst_x")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vdstX = existingVlayer.VdstX
		}
	}

	vdstY, err := getFloat64FromJson(mLayer, "vdst_y")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vdstY = existingVlayer.VdstY
		}
	}

	vdstW, err := getFloat64FromJson(mLayer, "vdst_w")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vdstW = existingVlayer.VdstW
		}
	}

	vdstH, err := getFloat64FromJson(mLayer, "vdst_h")
	if err != nil {
		if existingVlayer == nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		} else {
			vdstH = existingVlayer.VdstH
		}
	}

	if virtualW < 0 || virtualH < 0 {
		return nil, errors.New("virtual_w and virtual_h should not have negative values")
	}

	if vsrcX < 0 || vsrcY < 0 || vsrcW < 0 || vsrcH < 0 {
		return nil, errors.New("vsrc regions should not have negative values")
	}

	if vdstW < 0 || vdstH < 0 {
		return nil, errors.New("vdst_w and vdst_h should not have negative values")
	}

	if vdstW == 0 || vdstH == 0 {
		return nil, errors.New("vdst_w and vdst_h need to have values other than 0")
	}

	defVisibility := 1
	if existingVlayer != nil && existingVlayer.Visibility != nil {
		defVisibility = *existingVlayer.Visibility
	}
	visibility, err := getIntFromJsonDef(mLayer, "visibility", defVisibility)
	if err != nil {
		ELog.Println("error in generateLayerFromParam")
		return nil, err
	}

	vsurfaces := make([]layoutcore.VirtualSurface, 0)
	if genSurfaces {
		surfaces, err := getSliceFromJson(mLayer, "vsurface")
		if err != nil {
			ELog.Println("error in generateLayerFromParam")
			return nil, err
		}

		for _, mSurface := range surfaces {
			newVsurface, err := generateSurfaceFromParam(layerId, mSurface.(map[string]interface{}), appli_name)
			if err != nil {
				ELog.Println("error in generateLayerFromParam")
				return nil, err
			}
			vsurfaces = append(vsurfaces, *newVsurface)
		}
	}

	vlayer := layoutcore.VirtualLayer{
		AppName:    appli_name,
		VID:        layerId,
		Coord:      coord,
		VDisplayId: vdisplayId,
		VirtualW:   virtualW,
		VirtualH:   virtualH,
		VsrcX:      vsrcX,
		VsrcY:      vsrcY,
		VsrcW:      vsrcW,
		VsrcH:      vsrcH,
		VdstX:      vdstX,
		VdstY:      vdstY,
		VdstW:      vdstW,
		VdstH:      vdstH,
		Visibility: &visibility,
		Vsurfaces:  vsurfaces,
	}

	return &vlayer, nil
}

func modifySurfaceFromParam(modifySurface *layoutcore.VirtualSurface, mSurface map[string]interface{}) error {

	surfaceId, err := getIntFromJson(mSurface, "VID")
	if err != nil {
		return err
	}

	if surfaceId != modifySurface.VID {
		return errors.New("Error in modifySurfaceFromParam")
	}

	pixelW, err := getFloat64FromJson(mSurface, "pixel_w")
	if err == nil {
		modifySurface.PixelW = pixelW
	}

	pixelH, err := getFloat64FromJson(mSurface, "pixel_h")
	if err == nil {
		modifySurface.PixelH = pixelH
	}

	psrcX, err := getFloat64FromJson(mSurface, "psrc_x")
	if err == nil {
		modifySurface.PsrcX = psrcX
	}

	psrcY, err := getFloat64FromJson(mSurface, "psrc_y")
	if err == nil {
		modifySurface.PsrcY = psrcY
	}

	psrcW, err := getFloat64FromJson(mSurface, "psrc_w")
	if err == nil {
		modifySurface.PsrcW = psrcW
	}

	psrcH, err := getFloat64FromJson(mSurface, "psrc_h")
	if err == nil {
		modifySurface.PsrcH = psrcH
	}

	vdstX, err := getFloat64FromJson(mSurface, "vdst_x")
	if err == nil {
		modifySurface.VdstX = vdstX
	}

	vdstY, err := getFloat64FromJson(mSurface, "vdst_y")
	if err == nil {
		modifySurface.VdstY = vdstY
	}

	vdstW, err := getFloat64FromJson(mSurface, "vdst_w")
	if err == nil {
		modifySurface.VdstW = vdstW
	}

	vdstH, err := getFloat64FromJson(mSurface, "vdst_h")
	if err == nil {
		modifySurface.VdstH = vdstH
	}

	visibility, err := getIntFromJson(mSurface, "visibility")
	if err == nil {
		modifySurface.Visibility = &visibility
	}

	wlSurfaceId, err := getIntFromJson(mSurface, "wl_surface_id")
	if err == nil {
		modifySurface.WlSurfaceId = wlSurfaceId
	}

	return nil
}

func modifyLayerFromParam(modifyLayer *layoutcore.VirtualLayer, mLayer map[string]interface{}) error {

	layerId, err := getIntFromJson(mLayer, "VID")
	if err != nil {
		return err
	}

	if layerId != modifyLayer.VID {
		return errors.New("Error in modifyLayerFromParam")
	}

	coord, err := getCoordFromJson(mLayer, "coord")
	if err == nil {
		modifyLayer.Coord = coord
	}

	vdisplayId, err := getIntFromJsonDef(mLayer, "vdisplay_id", -1)
	if err == nil {
		modifyLayer.VDisplayId = vdisplayId
	}

	virtualW, err := getFloat64FromJson(mLayer, "virtual_w")
	if err == nil {
		modifyLayer.VirtualW = virtualW
	}

	virtualH, err := getFloat64FromJson(mLayer, "virtual_h")
	if err == nil {
		modifyLayer.VirtualH = virtualH
	}

	vsrcX, err := getFloat64FromJson(mLayer, "vsrc_x")
	if err == nil {
		modifyLayer.VsrcX = vsrcX
	}

	vsrcY, err := getFloat64FromJson(mLayer, "vsrc_y")
	if err == nil {
		modifyLayer.VsrcY = vsrcY
	}

	vsrcW, err := getFloat64FromJson(mLayer, "vsrc_w")
	if err == nil {
		modifyLayer.VsrcW = vsrcW
	}

	vsrcH, err := getFloat64FromJson(mLayer, "vsrc_h")
	if err == nil {
		modifyLayer.VsrcH = vsrcH
	}

	vdstX, err := getFloat64FromJson(mLayer, "vdst_x")
	if err == nil {
		modifyLayer.VdstX = vdstX
	}

	vdstY, err := getFloat64FromJson(mLayer, "vdst_y")
	if err == nil {
		modifyLayer.VdstY = vdstY
	}

	vdstW, err := getFloat64FromJson(mLayer, "vdst_w")
	if err == nil {
		modifyLayer.VdstW = vdstW
	}

	vdstH, err := getFloat64FromJson(mLayer, "vdst_h")
	if err == nil {
		modifyLayer.VdstH = vdstH
	}

	visibility, err := getIntFromJson(mLayer, "visibility")
	if err == nil {
		modifyLayer.Visibility = &visibility
	}

	return nil
}
