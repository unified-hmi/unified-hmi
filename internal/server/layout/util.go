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
	layoutcore "unified-hmi/internal/layout/core"
)

func dupVirtualLayerSlice(src []layoutcore.VirtualLayer) []layoutcore.VirtualLayer {
	dst := make([]layoutcore.VirtualLayer, 0, len(src))
	for _, layer := range src {
		copied := layer.Dup()
		dst = append(dst, *copied)
	}
	return dst
}
