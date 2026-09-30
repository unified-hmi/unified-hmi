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
	"os"
	"sync"
	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
)

const (
	windowOrderPolicyFile = "window_order_policy.json"
)

// PriorityConfigHolder stores a *PriorityConfig for concurrent access. It is
// guarded by a mutex instead of atomic.Pointer[T] so the code also builds on
// Go versions before 1.19. Load/Store mirror the atomic.Pointer API so callers
// are unaffected by the underlying implementation.
type PriorityConfigHolder struct {
	mu  sync.RWMutex
	cfg *PriorityConfig
}

func (h *PriorityConfigHolder) Load() *PriorityConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *PriorityConfigHolder) Store(cfg *PriorityConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = cfg
}

// LayerKey uniquely identifies a layer by application name and draw area name.
type LayerKey struct {
	AppName  string
	AreaName string
}

// PriorityGroupLayer represents a single layer entry in system.json.
type PriorityGroupLayer struct {
	AppName  string `json:"application_name"`
	AreaName string `json:"draw_area_name"`
}

// PriorityGroup represents a named group with an ordered list of layers.
type PriorityGroup struct {
	GroupName string               `json:"group_name"`
	Layers    []PriorityGroupLayer `json:"layers"`
}

// PriorityConfig holds the entire system.json priority configuration.
type PriorityConfig struct {
	DefaultGroup   string          `json:"default_group"`
	PriorityGroups []PriorityGroup `json:"priority_groups"`
}

// GetGroupPriority returns the priority index for a group name.
// Lower index = lower priority (further back). Returns -1 if not found.
func (pc *PriorityConfig) GetGroupPriority(groupName string) int {
	for i, g := range pc.PriorityGroups {
		if g.GroupName == groupName {
			return i
		}
	}
	return -1
}

// GetLayerGroup returns the group name for a given layer key.
// Matching is done in three passes:
//  1. Exact match on both AppName and AreaName.
//  2. App-name-only match: a policy entry with an empty draw_area_name acts as
//     a wildcard that matches any layer of the same application.
//  3. Any entry for the same app (covers dynamic areas like mirrors that are
//     not listed in the policy, ensuring they inherit the app's group).
//
// If no pass finds a match, the default group is returned.
func (pc *PriorityConfig) GetLayerGroup(key LayerKey) string {
	for _, g := range pc.PriorityGroups {
		for _, l := range g.Layers {
			if l.AppName == key.AppName && l.AreaName == key.AreaName {
				return g.GroupName
			}
		}
	}
	for _, g := range pc.PriorityGroups {
		for _, l := range g.Layers {
			if l.AppName == key.AppName && l.AreaName == "" {
				return g.GroupName
			}
		}
	}
	for _, g := range pc.PriorityGroups {
		for _, l := range g.Layers {
			if l.AppName == key.AppName {
				return g.GroupName
			}
		}
	}
	return pc.DefaultGroup
}

// GetGroupLayers returns the ordered list of LayerKeys for a group.
func (pc *PriorityConfig) GetGroupLayers(groupName string) []LayerKey {
	for _, g := range pc.PriorityGroups {
		if g.GroupName == groupName {
			keys := make([]LayerKey, 0, len(g.Layers))
			for _, l := range g.Layers {
				keys = append(keys, LayerKey{AppName: l.AppName, AreaName: l.AreaName})
			}
			return keys
		}
	}
	return nil
}

const (
	defaultPrimaryGroup = "Primary"
	defaultGeneralGroup = "General"
)

type appListPrimaryEntry struct {
	AppName string `json:"app_name"`
	Primary *bool  `json:"primary"`
}

type appListDefForPriority struct {
	Apps []appListPrimaryEntry `json:"apps"`
}

func buildDefaultPriorityConfigFromAppList() *PriorityConfig {
	path := config.AppListDefPath()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		ILog.Printf("buildDefaultPriorityConfigFromAppList: cannot read %s: %v; using empty two-group config", path, err)
		return &PriorityConfig{
			DefaultGroup: defaultGeneralGroup,
			PriorityGroups: []PriorityGroup{
				{GroupName: defaultGeneralGroup, Layers: []PriorityGroupLayer{}},
				{GroupName: defaultPrimaryGroup, Layers: []PriorityGroupLayer{}},
			},
		}
	}

	var def appListDefForPriority
	if err := json.Unmarshal(data, &def); err != nil {
		ILog.Printf("buildDefaultPriorityConfigFromAppList: parse error %s: %v; using empty two-group config", path, err)
		return &PriorityConfig{
			DefaultGroup: defaultGeneralGroup,
			PriorityGroups: []PriorityGroup{
				{GroupName: defaultGeneralGroup, Layers: []PriorityGroupLayer{}},
				{GroupName: defaultPrimaryGroup, Layers: []PriorityGroupLayer{}},
			},
		}
	}

	var primaryLayers, generalLayers []PriorityGroupLayer
	for _, app := range def.Apps {
		layer := PriorityGroupLayer{AppName: app.AppName}
		if app.Primary != nil && *app.Primary {
			primaryLayers = append(primaryLayers, layer)
		} else {
			generalLayers = append(generalLayers, layer)
		}
	}

	ILog.Printf("buildDefaultPriorityConfigFromAppList: Primary=%d apps, General=%d apps",
		len(primaryLayers), len(generalLayers))
	return &PriorityConfig{
		DefaultGroup: defaultGeneralGroup,
		PriorityGroups: []PriorityGroup{
			{GroupName: defaultGeneralGroup, Layers: generalLayers},
			{GroupName: defaultPrimaryGroup, Layers: primaryLayers},
		},
	}
}

// ReadPriorityConfig reads the policy JSON file and returns the priority
// configuration. If the file does not exist, a default two-group config is
// built from app-list-def.json (no error) so layers are grouped into General/Primary.
func ReadPriorityConfig() (*PriorityConfig, error) {
	systemJsonDir := config.LayoutSystemJSONDir()
	fname := systemJsonDir + string(os.PathSeparator) + windowOrderPolicyFile
	rdata, err := ioutil.ReadFile(fname)
	if err != nil {
		if os.IsNotExist(err) {
			ILog.Printf("Priority config not found at %s: building default two-group config from app-list-def.json", fname)
			return buildDefaultPriorityConfigFromAppList(), nil
		}
		WLog.Printf("Priority config read error at %s: %v", fname, err)
		return nil, err
	}

	pc := new(PriorityConfig)
	err = json.Unmarshal(rdata, pc)
	if err != nil {
		ELog.Printf("Failed to parse priority config %s: %v", fname, err)
		return nil, err
	}

	if pc.DefaultGroup == "" {
		return nil, errors.New("default_group is not specified in " + fname)
	}

	if pc.GetGroupPriority(pc.DefaultGroup) < 0 {
		return nil, errors.New("default_group '" + pc.DefaultGroup + "' not found in priority_groups")
	}

	ILog.Printf("Loaded priority config: %d groups, default_group=%s", len(pc.PriorityGroups), pc.DefaultGroup)
	return pc, nil
}
