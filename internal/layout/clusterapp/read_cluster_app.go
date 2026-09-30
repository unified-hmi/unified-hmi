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

package layoutclusterapp

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/core"
	. "unified-hmi/internal/ulog"
)

var DrawAreas []layoutcore.DrawArea
var AnimationSettings []layoutcore.AnimationSetting

const (
	initialLayout     = "initial_layout.json"
	appDrawArea       = "app_draw_area.json"
	animationSettings = "animation_settings.json"
)

type cfgInitialLayoutVsurface struct {
	VID         int     `json:"VID"`
	PixelW      float64 `json:"pixel_w"`
	PixelH      float64 `json:"pixel_h"`
	PsrcX       float64 `json:"psrc_x"`
	PsrcY       float64 `json:"psrc_y"`
	PsrcW       float64 `json:"psrc_w"`
	PsrcH       float64 `json:"psrc_h"`
	VdstX       float64 `json:"vdst_x"`
	VdstY       float64 `json:"vdst_y"`
	VdstW       float64 `json:"vdst_w"`
	VdstH       float64 `json:"vdst_h"`
	Visibility  *int    `json:"visibility"`
	WlSurfaceId int     `json:"wl_surface_id"`
}

type cfgInitialLayoutVLayer struct {
	VID        int                        `json:"VID"`
	Coord      *string                    `json:"coord"`
	VdisplayId *int                       `json:"vdisplay_id"` /* pointer type for check json property existence */
	Zorder     *int                       `json:"z_order"`     /* pointer type for check json property existence */
	VirtualW   float64                    `json:"virtual_w"`
	VirtualH   float64                    `json:"virtual_h"`
	VsrcX      float64                    `json:"vsrc_x"`
	VsrcY      float64                    `json:"vsrc_y"`
	VsrcW      float64                    `json:"vsrc_w"`
	VsrcH      float64                    `json:"vsrc_h"`
	VdstX      float64                    `json:"vdst_x"`
	VdstY      float64                    `json:"vdst_y"`
	VdstW      float64                    `json:"vdst_w"`
	VdstH      float64                    `json:"vdst_h"`
	Visibility *int                       `json:"visibility"`
	Vsurface   []cfgInitialLayoutVsurface `json:"vsurface"`
}

type cfgInitialLayout struct {
	AppName string                   `json:"application_name"`
	Vlayer  []cfgInitialLayoutVLayer `json:"vlayer"`
}

type cfgDrawArea struct {
	AppName string `json:"application_name"`
	Cutout  []struct {
		PixelW            int `json:"pixel_w"`
		PixelH            int `json:"pixel_h"`
		DrawingAreaSource int `json:"drawing_area_source"`
		Regions           []struct {
			RegionId   int    `json:"region_id"`
			VsurfaceId int    `json:"vsurface_id"`
			RegionName string `json:"region_name"`
			ScanoutX   int    `json:"scanout_x"`
			ScanoutY   int    `json:"scanout_y"`
			ScanoutW   int    `json:"scanout_w"`
			ScanoutH   int    `json:"scanout_h"`
		} `json:"regions"`
	} `json:"cutout"`
	Relations []struct {
		DrawId       int    `json:"draw_id"`
		DrawAreaName string `json:"draw_area_name"`
		VlayerId     int    `json:"vlayer_id"`
		RegionId     int    `json:"region_id"`
	} `json:"relations"`
}

type cfgWindowOrder struct {
	Order       string `json:"order"`
	RefAppName  string `json:"ref_app_name"`
	RefAreaName string `json:"ref_area_name"`
}

type cfgAnimationSetting struct {
	PatternName string `json:"pattern_name"`
	AppName     string `json:"application_name"`
	DrawId      int    `json:"draw_id"`
	Timeline    []struct {
		TimeFrame float64 `json:"time_frame"`
		Curve     struct {
			X0 float64 `json:"x0"`
			Y0 float64 `json:"y0"`
			X1 float64 `json:"x1"`
			Y1 float64 `json:"y1"`
		}
		VdstX       float64         `json:"vdst_x"`
		VdstY       float64         `json:"vdst_y"`
		VdstW       float64         `json:"vdst_w"`
		VdstH       float64         `json:"vdst_h"`
		VsrcX       float64         `json:"vsrc_x"`
		VsrcY       float64         `json:"vsrc_y"`
		VsrcW       float64         `json:"vsrc_w"`
		VsrcH       float64         `json:"vsrc_h"`
		WindowOrder *cfgWindowOrder `json:"window_order"`
	} `json:"timeline"`
}

func getLayoutClusterAppDirs() []string {
	var paths []string
	layoutPath := config.LayoutConfigDir()
	files, err := ioutil.ReadDir(layoutPath)
	if err != nil {
		return paths
	}

	for _, file := range files {
		if file.IsDir() == false {
			continue
		}
		paths = append(paths, filepath.Join(layoutPath, file.Name()))
	}

	return paths
}

func readAppInitialLayout(fname string) (*cfgInitialLayout, error) {

	rdata, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	ilt := new(cfgInitialLayout)
	err = json.Unmarshal(rdata, ilt)
	if err != nil {
		return nil, err
	}

	return ilt, nil
}

func layoutFromCfgInitialLayout(ilt *cfgInitialLayout) *layoutcore.Layout {
	layout := &layoutcore.Layout{Vlayers: make([]layoutcore.VirtualLayer, 0)}
	if ilt == nil {
		return layout
	}
	appName := ilt.AppName

	for _, r := range ilt.Vlayer {
		var (
			vdisplayId int
			coord      string
			zorder     int
		)

		if r.Zorder != nil {
			zorder = *r.Zorder
		}

		if r.Coord == nil || *r.Coord == layoutcore.COORD_GLOBAL {
			coord = layoutcore.COORD_GLOBAL
		} else if *r.Coord == layoutcore.COORD_VDISPLAY {
			coord = layoutcore.COORD_VDISPLAY
			if r.VdisplayId == nil {
				WLog.Printf("VID %d will be skipped because vdisplay_id is not specified", r.VID)
				continue
			}
			vdisplayId = *r.VdisplayId
		} else {
			WLog.Printf("VID %d will be skipped because coord is not specified", r.VID)
			continue
		}

		vlayer := layoutcore.VirtualLayer{
			AppName:    appName,
			VID:        r.VID,
			ZOrder:     zorder,
			Coord:      coord,
			VDisplayId: vdisplayId,
			VirtualW:   r.VirtualW,
			VirtualH:   r.VirtualH,
			VsrcX:      r.VsrcX,
			VsrcY:      r.VsrcY,
			VsrcW:      r.VsrcW,
			VsrcH:      r.VsrcH,
			VdstX:      r.VdstX,
			VdstY:      r.VdstY,
			VdstW:      r.VdstW,
			VdstH:      r.VdstH,
			Visibility: r.Visibility,
			Vsurfaces:  make([]layoutcore.VirtualSurface, 0),
		}
		for _, s := range r.Vsurface {
			vsurf := layoutcore.VirtualSurface{
				AppName:     appName,
				ParentVID:   r.VID,
				VID:         s.VID,
				PixelW:      s.PixelW,
				PixelH:      s.PixelH,
				PsrcX:       s.PsrcX,
				PsrcY:       s.PsrcY,
				PsrcW:       s.PsrcW,
				PsrcH:       s.PsrcH,
				VdstX:       s.VdstX,
				VdstY:       s.VdstY,
				VdstW:       s.VdstW,
				VdstH:       s.VdstH,
				Visibility:  s.Visibility,
				WlSurfaceId: s.WlSurfaceId,
			}
			vlayer.Vsurfaces = append(vlayer.Vsurfaces, vsurf)
		}
		layout.Vlayers = append(layout.Vlayers, vlayer)
	}
	return layout
}

func newLayoutFromCfg(appDir string) (*layoutcore.Layout, error) {
	fname := filepath.Join(appDir, initialLayout)
	ilt, err := readAppInitialLayout(fname)
	if err != nil {
		return nil, err
	}
	return layoutFromCfgInitialLayout(ilt), nil
}

func LayoutTreeFromInitialLayoutJSON(data []byte) (string, *layoutcore.LayoutTree, error) {
	var ilt cfgInitialLayout
	if err := json.Unmarshal(data, &ilt); err != nil {
		return "", nil, err
	}
	if ilt.AppName == "" {
		return "", nil, errors.New("application_name is required")
	}
	layout := layoutFromCfgInitialLayout(&ilt)
	if len(layout.Vlayers) == 0 {
		return "", nil, errors.New("vlayer is required")
	}
	tree := &layoutcore.LayoutTree{Vlayers: make([]layoutcore.VirtualLayer, 0)}
	insertLayoutToLayoutTree(tree, layout)
	return ilt.AppName, tree, nil
}

func insertLayoutToLayoutTree(tree *layoutcore.LayoutTree, nlayout *layoutcore.Layout) {
	for _, nvlayer := range nlayout.Vlayers {

		if len(tree.Vlayers) == 0 {
			tree.Vlayers = append(tree.Vlayers, *nvlayer.Dup())
			continue
		}

		inserted := false

		for idx, r := range tree.Vlayers {
			if r.ZOrder > nvlayer.ZOrder {
				tree.Vlayers = append(tree.Vlayers[:idx+1], tree.Vlayers[idx:]...)
				tree.Vlayers[idx] = *nvlayer.Dup()
				inserted = true
				break
			}
		}

		if inserted {
			continue
		}

		tree.Vlayers = append(tree.Vlayers, *nvlayer.Dup())
	}
}

func ReadLayoutTreeFromCfg() (*layoutcore.LayoutTree, error) {

	layoutTree := &layoutcore.LayoutTree{
		Vlayers: make([]layoutcore.VirtualLayer, 0),
	}

	appDirs := getLayoutClusterAppDirs()

	DLog.Println(appDirs)

	for _, appDir := range appDirs {
		layout, err := newLayoutFromCfg(appDir)
		if err != nil {
			WLog.Printf("newLayoutFromCfg %s error: %s\n", appDir, err)
			continue
		}
		insertLayoutToLayoutTree(layoutTree, layout)
	}

	if len(layoutTree.Vlayers) == 0 {
		return layoutTree, errors.New("Cannot read layout config")
	}

	return layoutTree, nil
}

func ReadLayoutTreeFromCfgForApp(appName string) (*layoutcore.LayoutTree, error) {
	layoutTree := &layoutcore.LayoutTree{
		Vlayers: make([]layoutcore.VirtualLayer, 0),
	}

	if appName == "" {
		return layoutTree, errors.New("appName is empty")
	}

	appDirs := getLayoutClusterAppDirs()

	for _, appDir := range appDirs {
		fname := filepath.Join(appDir, initialLayout)
		ilt, err := readAppInitialLayout(fname)
		if err != nil {
			continue
		}

		if ilt.AppName != appName {
			continue
		}

		layout, err := newLayoutFromCfg(appDir)
		if err != nil {
			return layoutTree, err
		}

		insertLayoutToLayoutTree(layoutTree, layout)
		if len(layoutTree.Vlayers) == 0 {
			return layoutTree, errors.New("Cannot read layout config")
		}

		return layoutTree, nil
	}

	return layoutTree, errors.New("Cannot read layout config for app")
}

func readAppDrawArea(fname string) (*cfgDrawArea, error) {
	rdata, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	da := new(cfgDrawArea)
	err = json.Unmarshal(rdata, da)

	if err != nil {
		return nil, err
	}
	return da, nil
}

func newRelationsFromCfg(appDir string) (*layoutcore.DrawArea, error) {
	fname := filepath.Join(appDir, appDrawArea)

	rda, err := readAppDrawArea(fname)
	if err != nil {
		return nil, err
	}

	appName := rda.AppName

	da := layoutcore.DrawArea{
		AppName:   appName,
		Relations: make([]layoutcore.Relation, 0),
		Cutout:    make([]layoutcore.Cutout, 0),
	}

	for _, r := range rda.Relations {
		relation := layoutcore.Relation{
			DrawId:       r.DrawId,
			DrawAreaName: r.DrawAreaName,
			VLayerId:     r.VlayerId,
			RegionId:     r.RegionId,
		}
		da.Relations = append(da.Relations, relation)
	}

	for _, c := range rda.Cutout {
		regions := make([]layoutcore.Region, 0)
		for _, re := range c.Regions {
			region := layoutcore.Region{
				RegionId:   re.RegionId,
				VsurfaceId: re.VsurfaceId,
				RegionName: re.RegionName,
				ScanoutX:   re.ScanoutX,
				ScanoutY:   re.ScanoutY,
				ScanoutW:   re.ScanoutW,
				ScanoutH:   re.ScanoutH,
			}
			regions = append(regions, region)
		}
		cutout := layoutcore.Cutout{
			PixelW:            c.PixelW,
			PixelH:            c.PixelH,
			DrawingAreaSource: c.DrawingAreaSource,
			Regions:           regions,
		}
		da.Cutout = append(da.Cutout, cutout)
	}

	return &da, nil
}

func readAnimationSettings(fname string) ([]cfgAnimationSetting, error) {
	rdata, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	ass := make([]cfgAnimationSetting, 0)
	err = json.Unmarshal(rdata, &ass)
	if err != nil {
		return nil, err
	}
	return ass, nil
}

func newAnimationSettingsFromCfg(appDir string) ([]layoutcore.AnimationSetting, error) {
	fname := filepath.Join(appDir, animationSettings)
	assJson, err := readAnimationSettings(fname)
	if err != nil {
		return nil, err
	}

	ass := make([]layoutcore.AnimationSetting, 0)

	for _, asJson := range assJson {
		timelines := make([]layoutcore.TimeLine, 0)
		for _, tl := range asJson.Timeline {
			var timeline layoutcore.TimeLine
			timeline.TimeFrame = tl.TimeFrame
			timeline.Curve.X0 = tl.Curve.X0
			timeline.Curve.Y0 = tl.Curve.Y0
			timeline.Curve.X1 = tl.Curve.X1
			timeline.Curve.Y1 = tl.Curve.Y1
			timeline.VdstX = tl.VdstX
			timeline.VdstY = tl.VdstY
			timeline.VdstW = tl.VdstW
			timeline.VdstH = tl.VdstH
			timeline.VsrcX = tl.VsrcX
			timeline.VsrcY = tl.VsrcY
			timeline.VsrcW = tl.VsrcW
			timeline.VsrcH = tl.VsrcH
			if tl.WindowOrder != nil {
				timeline.WindowOrder = &layoutcore.WindowOrderSetting{
					Order:      tl.WindowOrder.Order,
					RefAppName: tl.WindowOrder.RefAppName,
					RefArea:    tl.WindowOrder.RefAreaName,
				}
			}
			timelines = append(timelines, timeline)
		}
		as := layoutcore.AnimationSetting{
			PatternName: asJson.PatternName,
			AppName:     asJson.AppName,
			DrawId:      asJson.DrawId,
			TimeLine:    timelines,
		}
		ass = append(ass, as)
	}

	return ass, nil
}

func ClusterAppConfigure() {
	appDirs := getLayoutClusterAppDirs()
	DrawAreas = make([]layoutcore.DrawArea, 0)
	AnimationSettings = make([]layoutcore.AnimationSetting, 0)
	for _, appDir := range appDirs {
		da, err := newRelationsFromCfg(appDir)
		if err != nil {
			WLog.Printf("%s/%s read error: %s", appDir, appDrawArea, err)
		} else {
			DrawAreas = append(DrawAreas, *da)
		}

		ass, err := newAnimationSettingsFromCfg(appDir)
		if err != nil {
			WLog.Printf("%s/%s read error: %s", appDir, animationSettings, err)
		} else {
			for _, as := range ass {
				AnimationSettings = append(AnimationSettings, as)
			}
		}
	}

	if len(DrawAreas) == 0 {
		WLog.Println("Draw area information is empty and cannot be configured")
	} else {
		DLog.Println("Draw area information: ", DrawAreas)
	}

	if len(AnimationSettings) == 0 {
		WLog.Println("Animation settings are empty and cannot be configured")
	} else {
		DLog.Println("Animation settings: ", AnimationSettings)
	}
}

// LookupAreaNameByVID returns the DrawAreaName of the DrawAreas relation for
// appName whose VLayerId matches vid, or "" if none matches.
func LookupAreaNameByVID(appName string, vid int) string {
	for _, drawArea := range DrawAreas {
		if drawArea.AppName == appName {
			for _, relation := range drawArea.Relations {
				if relation.VLayerId == vid {
					return relation.DrawAreaName
				}
			}
		}
	}
	return ""
}
