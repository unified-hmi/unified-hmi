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
	"sort"

	layoutclusterapp "unified-hmi/internal/layout/clusterapp"
	layoutcommgen "unified-hmi/internal/layout/commgen"
	layoutcore "unified-hmi/internal/layout/core"
	layoutmulticonn "unified-hmi/internal/layout/multiconn"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	. "unified-hmi/internal/ulog"
)

type windowOrderConstraint struct {
	Order  layoutcore.InsertOrder
	RefKey layoutclusterapp.LayerKey // unused for prepend/append
	seq    uint64
}

func layerKeyOf(layer layoutcore.VirtualLayer) layoutclusterapp.LayerKey {
	return layoutclusterapp.LayerKey{AppName: layer.AppName, AreaName: layer.AreaName}
}

func indexOfLayerKey(layers []layoutcore.VirtualLayer, key layoutclusterapp.LayerKey) int {
	for i, l := range layers {
		if layerKeyOf(l) == key {
			return i
		}
	}
	return -1
}

func insertLayerAt(layers []layoutcore.VirtualLayer, pos int, layer layoutcore.VirtualLayer) []layoutcore.VirtualLayer {
	if pos < 0 {
		pos = 0
	}
	if pos > len(layers) {
		pos = len(layers)
	}
	layers = append(layers, layoutcore.VirtualLayer{})
	copy(layers[pos+1:], layers[pos:])
	layers[pos] = layer
	return layers
}

func (s *Server) setWindowOrderConstraint(key layoutclusterapp.LayerKey, c windowOrderConstraint) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	if s.orderConstraints == nil {
		s.orderConstraints = make(map[layoutclusterapp.LayerKey]windowOrderConstraint)
	}
	s.orderSeq++
	c.seq = s.orderSeq
	s.orderConstraints[key] = c
}

func (s *Server) removeWindowOrderConstraint(key layoutclusterapp.LayerKey) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	delete(s.orderConstraints, key)
}

func (s *Server) clearWindowOrderState() {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	s.orderConstraints = make(map[layoutclusterapp.LayerKey]windowOrderConstraint)
	s.orderSnapshot = nil
	s.orderSeq = 0
}

func (s *Server) seedWindowOrderSnapshot(layers []layoutcore.VirtualLayer) {
	keys := make([]layoutclusterapp.LayerKey, 0, len(layers))
	for _, l := range layers {
		keys = append(keys, layerKeyOf(l))
	}
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	s.orderSnapshot = keys
}

func (s *Server) computeDesiredOrder() []layoutcore.VirtualLayer {
	base := s.getAllCachedLayersSorted()
	if len(base) == 0 {
		return base
	}

	type keyedConstraint struct {
		key layoutclusterapp.LayerKey
		c   windowOrderConstraint
	}
	s.orderMu.Lock()
	constraints := make([]keyedConstraint, 0, len(s.orderConstraints))
	for k, v := range s.orderConstraints {
		constraints = append(constraints, keyedConstraint{key: k, c: v})
	}
	s.orderMu.Unlock()

	if len(constraints) == 0 {
		return base
	}
	sort.Slice(constraints, func(i, j int) bool {
		return constraints[i].c.seq < constraints[j].c.seq
	})

	order := append([]layoutcore.VirtualLayer(nil), base...)
	for _, kc := range constraints {
		order = s.applyOrderConstraint(order, kc.key, kc.c)
	}
	return order
}

func (s *Server) applyOrderConstraint(order []layoutcore.VirtualLayer, key layoutclusterapp.LayerKey, c windowOrderConstraint) []layoutcore.VirtualLayer {
	idx := indexOfLayerKey(order, key)
	if idx < 0 {
		return order
	}
	layer := order[idx]

	switch c.Order {
	case layoutcore.InsertBefore, layoutcore.InsertAfter:
		if c.RefKey == key || indexOfLayerKey(order, c.RefKey) < 0 {
			return order
		}
		order = append(order[:idx], order[idx+1:]...)
		refIdx := indexOfLayerKey(order, c.RefKey)
		if c.Order == layoutcore.InsertAfter {
			refIdx++
		}
		return insertLayerAt(order, refIdx, layer)

	case layoutcore.InsertPrepend, layoutcore.InsertAppend:
		group := s.getCurrentGroup(key)
		order = append(order[:idx], order[idx+1:]...)
		first, last := -1, -1
		for i, l := range order {
			if s.getCurrentGroup(layerKeyOf(l)) != group {
				continue
			}
			if first < 0 {
				first = i
			}
			last = i
		}
		if first < 0 {
			return insertLayerAt(order, idx, layer)
		}
		if c.Order == layoutcore.InsertPrepend {
			return insertLayerAt(order, first, layer)
		}
		return insertLayerAt(order, last+1, layer)
	}
	return order
}

func indexOfVID(layers []layoutcore.VirtualLayer, vid int) int {
	for i, l := range layers {
		if l.VID == vid {
			return i
		}
	}
	return -1
}

func mergeDesiredWithLive(desired, live []layoutcore.VirtualLayer) []layoutcore.VirtualLayer {
	liveByVID := make(map[int]layoutcore.VirtualLayer, len(live))
	for _, l := range live {
		liveByVID[l.VID] = l
	}

	placed := make(map[int]bool, len(live))
	out := make([]layoutcore.VirtualLayer, 0, len(live))
	for _, d := range desired {
		lv, ok := liveByVID[d.VID]
		if !ok {
			continue
		}
		out = append(out, lv)
		placed[lv.VID] = true
	}

	for i, l := range live {
		if placed[l.VID] {
			continue
		}
		pos := 0
		for j := i - 1; j >= 0; j-- {
			if !placed[live[j].VID] {
				continue
			}
			if k := indexOfVID(out, live[j].VID); k >= 0 {
				pos = k + 1
			}
			break
		}
		out = insertLayerAt(out, pos, l)
		placed[l.VID] = true
	}
	return out
}

func (s *Server) setVisibilityForLayers(layers []layoutcore.VirtualLayer, visibility int) error {
	for _, layer := range layers {
		vlayer, err := layoutvscreen.VScreen.GetVlayerParams(layer.VID)
		if err != nil {
			WLog.Printf("setVisibilityForLayers: VID=%d not in VScreen: %v", layer.VID, err)
			continue
		}
		v := visibility
		vlayer.Visibility = &v

		cmd, err := layoutcommgen.GenerateCommModifyVlayer(vlayer)
		if err != nil {
			return err
		}
		if err := layoutmulticonn.MulCon.SendLayoutCommand(cmd); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) publishDesiredOrder() error {
	desired := s.computeDesiredOrder()
	if len(desired) == 0 {
		return nil
	}

	live := layoutvscreen.VScreen.GetOrderedVlayers()
	if len(live) == 0 {
		return nil
	}

	ordered := mergeDesiredWithLive(desired, live)
	if len(ordered) == 0 {
		return nil
	}

	keys := make([]layoutclusterapp.LayerKey, 0, len(ordered))
	vids := make([]int, 0, len(ordered))
	for _, l := range ordered {
		keys = append(keys, layerKeyOf(l))
		vids = append(vids, l.VID)
	}

	s.orderMu.Lock()
	unchanged := layerKeysEqual(s.orderSnapshot, keys)
	if !unchanged {
		s.orderSnapshot = keys
	}
	s.orderMu.Unlock()
	if unchanged {
		return nil
	}

	cmd, err := layoutcommgen.GenerateCommSetVlayerOrder(vids)
	if err != nil {
		return err
	}
	if err := layoutmulticonn.MulCon.SendLayoutCommand(cmd); err != nil {
		WLog.Printf("publishDesiredOrder: send failed: %v", err)
		return err
	}
	DLog.Printf("publishDesiredOrder: published %d layers", len(ordered))
	return nil
}

func layerKeysEqual(a, b []layoutclusterapp.LayerKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
