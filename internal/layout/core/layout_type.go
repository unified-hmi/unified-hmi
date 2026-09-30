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

package layoutcore

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "unified-hmi/internal/ulog"
)

const (
	appListDefPathEnv     = "APP_LIST_DEF_PATH"
	appListDefConfigDir   = "UHMI_CONFIG_DIR"
	appListDefDefaultDir  = "/etc/uhmi-framework"
	appListDefDefaultName = "app-list-def.json"
)

func appListDefPath() string {
	if v := os.Getenv(appListDefPathEnv); v != "" {
		return v
	}
	configDir := os.Getenv(appListDefConfigDir)
	if configDir == "" {
		configDir = appListDefDefaultDir
	}
	return filepath.Join(configDir, appListDefDefaultName)
}

func RoundTo5(x float64) float64 {
	return math.Round(x*100000) / 100000
}

// ComputeAppLayerVID returns a layer VID for appName derived from a polynomial
// rolling hash (multiplier 31). This is intended for synthetic names that are
// not registered in app-list-def.json and are not mirror layers.
// TODO: temporary workaround until a proper VID management system is in place.
func ComputeAppLayerVID(appName string) int {
	var h uint32
	for _, c := range appName {
		h = h*31 + uint32(c)
	}
	return int(h%8000000) + 1000000
}

// ParseMirrorName checks whether name follows the "<originalApp>_mirror<N>"
// pattern produced by AllocMirrorName. On success it returns the original app
// name, the 0-based mirror index, and ok=true. On failure it returns ok=false.
func ParseMirrorName(name string) (originalApp string, mirrorIndex int, ok bool) {
	const sep = "_mirror"
	idx := strings.LastIndex(name, sep)
	if idx < 0 {
		return "", 0, false
	}
	suffix := name[idx+len(sep):]
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 0 {
		return "", 0, false
	}
	return name[:idx], n, true
}

// ComputeMirrorLayerVID returns the layer VID for a mirror as
// originalLayerVID + n, where n is the 1-based mirror number (mirrorIndex + 1).
// It resolves originalLayerVID via ComputeAppVIDs; if originalAppName is not
// listed in app-list-def.json an error is returned with no hash fallback.
func ComputeMirrorLayerVID(originalAppName string, n int) (int, error) {
	layerVID, _, err := ComputeAppVIDs(originalAppName)
	if err != nil {
		return 0, fmt.Errorf("ComputeMirrorLayerVID: %w", err)
	}
	return layerVID + n, nil
}

type AppListEntry struct {
	AppName    string `json:"app_name"`
	Sender     string `json:"sender"`
	AppCommand string `json:"app_command"`
	AppSize    struct {
		W int `json:"w"`
		H int `json:"h"`
	} `json:"app_size"`
	CommandEnv     string `json:"command_env"`
	ServerPort     int    `json:"server_port"`
	SessionTimeout int    `json:"session_timeout"`
	Primary        *bool  `json:"primary"`
}

type appListDef struct {
	Apps []AppListEntry `json:"apps"`
}

// ReadAppListEntries reads ordered entries from app-list-def.json.
func ReadAppListEntries() ([]AppListEntry, error) {
	path := appListDefPath()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("readAppListEntries: %w", err)
	}
	var def appListDef
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("readAppListEntries: failed to parse %s: %w", path, err)
	}
	return def.Apps, nil
}

func ReadAppListEntry(appName string) (*AppListEntry, error) {
	entries, err := ReadAppListEntries()
	if err != nil {
		return nil, fmt.Errorf("readAppListEntry: %w", err)
	}
	for i := range entries {
		if entries[i].AppName == appName {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// ResolveAppLayerVID returns the layer VID for appName, choosing the
// appropriate computation strategy:
//   - Regular apps (listed in app-list-def.json): position-based VID via
//     ComputeAppVIDs, matching the VID registered in VScreen at LaunchApp time.
//   - Mirror names ("<originalApp>_mirror<N>"): position-based VID of the
//     original app plus the 1-based mirror number, matching the VID registered
//     at CreateAppMirror time via ComputeMirrorLayerVID.
//   - Other synthetic names (not in the file, not a mirror): hash-based VID via
//     ComputeAppLayerVID.
//
// File I/O or JSON parse errors from ComputeAppVIDs are propagated as-is so
// that genuine configuration problems are not silently swallowed.
func ResolveAppLayerVID(appName string) (int, error) {
	layerVID, _, err := ComputeAppVIDs(appName)
	if err == nil {
		return layerVID, nil
	}
	path := appListDefPath()
	notFound := fmt.Sprintf("ComputeAppVIDs: app %q not found in %s", appName, path)
	if err.Error() != notFound {
		return 0, err
	}
	if originalApp, mirrorIndex, ok := ParseMirrorName(appName); ok {
		return ComputeMirrorLayerVID(originalApp, mirrorIndex+1)
	}
	return ComputeAppLayerVID(appName), nil
}

// ComputeAppVIDs returns the stable layerVID and surfaceVID for appName using
// the same strategy as LaunchApp:
//  1. Read app-list-def.json (env APP_LIST_DEF_PATH overrides the default path).
//  2. If found at 1-based position N: layerVID = 1000000 + N*10000,
//     surfaceVID = layerVID + 1000.
//
// Returns an error if the app is not listed in app-list-def.json. All WoJson
// operations (launch, move, widen) must use this function so that VIDs are
// always consistent with what was registered in VScreen at launch time.
func ComputeAppVIDs(appName string) (layerVID, surfaceVID int, err error) {
	path := appListDefPath()
	data, readErr := ioutil.ReadFile(path)
	if readErr != nil {
		return 0, 0, fmt.Errorf("ComputeAppVIDs: failed to read %s: %w", path, readErr)
	}
	var def appListDef
	if jsonErr := json.Unmarshal(data, &def); jsonErr != nil {
		return 0, 0, fmt.Errorf("ComputeAppVIDs: failed to parse %s: %w", path, jsonErr)
	}
	for i, a := range def.Apps {
		if a.AppName == appName {
			pos := i + 1
			layerVID = 1000000 + pos*10000
			surfaceVID = layerVID + 1000
			return layerVID, surfaceVID, nil
		}
	}
	return 0, 0, fmt.Errorf("ComputeAppVIDs: app %q not found in %s", appName, path)
}

func PrintLayoutTree(tree *LayoutTree) {
	DLog.Printf("LayoutTree:")
	for i, layer := range tree.Vlayers {
		vis := -1
		if layer.Visibility != nil {
			vis = *layer.Visibility
		}
		DLog.Printf("  Vlayer[%d] AppName=%-16s VID=%-8d Coord=%-10s\n            Virtual (%.0f x %.0f)  Vsrc=(%.0f,%.0f,%.0f,%.0f)  Vdst=(%.0f,%.0f,%.0f,%.0f)  Visibility=%d",
			i, layer.AppName, layer.VID, layer.Coord,
			layer.VirtualW, layer.VirtualH,
			layer.VsrcX, layer.VsrcY, layer.VsrcW, layer.VsrcH,
			layer.VdstX, layer.VdstY, layer.VdstW, layer.VdstH,
			vis)
		for j, surf := range layer.Vsurfaces {
			svis := -1
			if surf.Visibility != nil {
				svis = *surf.Visibility
			}
			DLog.Printf("    Vsurface[%d] AppName=%-16s VID=%-8d ParentVID=%d\n               Pixel (%.0f x %.0f)  Psrc=(%.0f,%.0f,%.0f,%.0f)  Vdst=(%.0f,%.0f,%.0f,%.0f)  Visibility=%d",
				j, surf.AppName, surf.VID, surf.ParentVID,
				surf.PixelW, surf.PixelH,
				surf.PsrcX, surf.PsrcY, surf.PsrcW, surf.PsrcH,
				surf.VdstX, surf.VdstY, surf.VdstW, surf.VdstH,
				svis)
		}
	}
}

const (
	COORD_GLOBAL   = "global"
	COORD_VDISPLAY = "vdisplay"
)

type VirtualDisplay struct {
	DispName   string
	VDisplayId int
	VirtualX   float64
	VirtualY   float64
	VirtualW   float64
	VirtualH   float64
}

type VirtualDisplayArea struct {
	AreaName string
	VirtualX float64
	VirtualY float64
	VirtualW float64
	VirtualH float64
}

type VirtualSurface struct {
	AppName string

	ParentVID int
	VID       int

	PixelW float64
	PixelH float64

	PsrcX float64
	PsrcY float64
	PsrcW float64
	PsrcH float64

	VdstX float64
	VdstY float64
	VdstW float64
	VdstH float64

	Visibility  *int
	WlSurfaceId int
}

type VirtualLayer struct {
	AppName   string
	AreaName  string /* draw area name for priority group lookup */
	GroupName string /* priority group name from system.json */

	VID    int
	ZOrder int

	Coord      string /* "global" or "vdisplay" */
	VDisplayId int    /* only used if Coord is "vdisplay" */

	VirtualW float64
	VirtualH float64

	VsrcX float64
	VsrcY float64
	VsrcW float64
	VsrcH float64

	VdstX float64
	VdstY float64
	VdstW float64
	VdstH float64

	Visibility *int

	Vsurfaces []VirtualSurface
}

type VirtualSafetyArea struct {
	VirtualX float64
	VirtualY float64
	VirtualW float64
	VirtualH float64
}

type WindowOrderSetting struct {
	Order      InsertOrder
	RefAppName string
	RefArea    string
}

type TimeLine struct {
	TimeFrame float64
	Curve     struct {
		X0 float64
		X1 float64
		Y0 float64
		Y1 float64
	}
	VdstX       float64
	VdstY       float64
	VdstW       float64
	VdstH       float64
	VsrcX       float64
	VsrcY       float64
	VsrcW       float64
	VsrcH       float64
	WindowOrder *WindowOrderSetting
}

type AnimationSetting struct {
	PatternName string
	AppName     string
	DrawId      int
	TimeLine    []TimeLine
}

type Relation struct {
	DrawId       int
	DrawAreaName string
	VLayerId     int
	RegionId     int
}

type Region struct {
	RegionId   int
	VsurfaceId int
	RegionName string
	ScanoutX   int
	ScanoutY   int
	ScanoutW   int
	ScanoutH   int
}

type Cutout struct {
	PixelW            int
	PixelH            int
	DrawingAreaSource int
	Regions           []Region
}

type DrawArea struct {
	AppName   string
	Relations []Relation
	Cutout    []Cutout
}

type Layout struct {
	Vlayers []VirtualLayer
}

type LayoutTree struct {
	Vlayers []VirtualLayer
}
