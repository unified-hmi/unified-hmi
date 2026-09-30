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

// InsertOrder specifies where a vlayer/vsurface is inserted relative to its
// existing siblings in the z-order. It is an alias of string so the constants
// remain compatible with the JSON-tagged protocol fields that carry them.
type InsertOrder = string

const (
	// InsertBefore places the element immediately before the reference VID.
	InsertBefore InsertOrder = "before"
	// InsertAfter places the element immediately after the reference VID.
	InsertAfter InsertOrder = "after"
	// InsertPrepend places the element at the front (bottom) of the list.
	InsertPrepend InsertOrder = "prepend"
	// InsertAppend places the element at the back (top) of the list.
	InsertAppend InsertOrder = "append"
)
