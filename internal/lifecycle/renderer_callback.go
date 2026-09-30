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

package lifecycle

import "sync"

type RendererCallback func(execCommInfo ExecCommInfo, commTaskCtx *CommTaskContext) error

var (
	rendererCallbackMu sync.RWMutex
	rendererCallback   RendererCallback
)

func SetRendererCallback(cb RendererCallback) {
	rendererCallbackMu.Lock()
	defer rendererCallbackMu.Unlock()
	rendererCallback = cb
}

func getRendererCallback() RendererCallback {
	rendererCallbackMu.RLock()
	defer rendererCallbackMu.RUnlock()
	return rendererCallback
}

func HasRendererCallback() bool {
	rendererCallbackMu.RLock()
	defer rendererCallbackMu.RUnlock()
	return rendererCallback != nil
}
