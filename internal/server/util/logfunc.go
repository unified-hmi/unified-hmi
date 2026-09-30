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

// Package serverutil holds tiny helpers shared across the internal/server
// domain packages (layoutserver, lifecycleserver, uhmiserver).
package serverutil

import (
	"runtime"

	. "unified-hmi/internal/ulog"
)

// LogFunc logs the name of its caller. Call it as the first line of an RPC
// handler to trace which handler ran.
func LogFunc() {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		WLog.Println("Could not get caller info")
		return
	}
	funcName := runtime.FuncForPC(pc).Name()
	DLog.Println("Function:", funcName)
}
