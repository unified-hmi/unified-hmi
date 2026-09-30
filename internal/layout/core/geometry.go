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

// LayersOverlap reports whether two axis-aligned rectangles overlap.
func LayersOverlap(ax, ay, aw, ah, bx, by, bw, bh float64) bool {
	return ax < bx+bw && ax+aw > bx && ay < by+bh && ay+ah > by
}

// DiffLayersByVID compares newLayers against oldLayers by VID and returns the
// VIDs present only in oldLayers (to remove), the layers present only in
// newLayers (to add) and the layers present in both (to modify).
func DiffLayersByVID(oldLayers, newLayers []VirtualLayer) (removeIDs []int, addLayers, modifyLayers []VirtualLayer) {
	oldByVID := make(map[int]VirtualLayer, len(oldLayers))
	for _, layer := range oldLayers {
		oldByVID[layer.VID] = layer
	}
	newByVID := make(map[int]VirtualLayer, len(newLayers))
	for _, layer := range newLayers {
		newByVID[layer.VID] = layer
	}
	removeIDs = make([]int, 0)
	for vid := range oldByVID {
		if _, ok := newByVID[vid]; !ok {
			removeIDs = append(removeIDs, vid)
		}
	}
	addLayers = make([]VirtualLayer, 0)
	modifyLayers = make([]VirtualLayer, 0)
	for _, layer := range newLayers {
		if _, ok := oldByVID[layer.VID]; ok {
			modifyLayers = append(modifyLayers, layer)
		} else {
			addLayers = append(addLayers, layer)
		}
	}
	return removeIDs, addLayers, modifyLayers
}
