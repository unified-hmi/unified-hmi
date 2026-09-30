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

// Package config centralizes all filesystem path and directory resolution used
// across the unified-hmi components. Every path can be relocated at runtime via
// environment variables so that the framework is not tied to the built-in
// Linux defaults (/etc/uhmi-framework, /var/local/uhmi-app, /tmp).
//
// Resolution order for every accessor is:
//  1. The item-specific environment variable, when set and non-empty.
//  2. A path derived from the relevant base directory (which itself honours a
//     base-directory environment variable) joined with the default file name.
//
// This package intentionally depends only on the standard library so that it
// can be imported from any other internal package without creating an import
// cycle.
package config

import (
	"os"
	"path/filepath"
)

const (
	defaultConfigDir  = "/etc/uhmi-framework"
	defaultAppDir     = "/var/local/uhmi-app"
	defaultRuntimeDir = "/tmp"

	layoutSubDir = "layout"

	appListDefName = "app-list-def.json"
	vscreenDefName = "virtual-screen-def.json"

	rvgpuLockName  = "rvgpu.lock"
	rvgpuIndexName = "rvgpu-index"
)

// Environment variable names recognised by this package.
const (
	// Base-directory overrides.
	EnvConfigDir  = "UHMI_CONFIG_DIR"
	EnvAppDir     = "UHMI_APP_DIR"
	EnvRuntimeDir = "UHMI_RUNTIME_DIR"

	// Item-specific overrides.
	EnvAppListDefPath      = "APP_LIST_DEF_PATH"
	EnvVScreenDefPath      = "VSDPATH"
	EnvLayoutConfigDir     = "LAYOUTPATH"
	EnvLayoutSystemJSONDir = "SYSTEMJSONPATH"
	EnvLifecycleAppDir     = "LIFECYCLEPATH"
	EnvRvgpuLockFile       = "RVGPU_LOCK_FILE"
	EnvRvgpuIndexFile      = "RVGPU_INDEX_FILE"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ConfigDir returns the base directory that holds the framework configuration
// files (app-list-def.json, virtual-screen-def.json).
// Override with UHMI_CONFIG_DIR.
func ConfigDir() string {
	return envOr(EnvConfigDir, defaultConfigDir)
}

// AppDir returns the base directory that holds per-application data such as the
// layout configuration subtree and the lifecycle app.json files.
// Override with UHMI_APP_DIR.
func AppDir() string {
	return envOr(EnvAppDir, defaultAppDir)
}

// RuntimeDir returns the directory used for runtime sockets and lock files.
// Override with UHMI_RUNTIME_DIR.
func RuntimeDir() string {
	return envOr(EnvRuntimeDir, defaultRuntimeDir)
}

// AppListDefPath returns the full path to app-list-def.json.
// Override with APP_LIST_DEF_PATH.
func AppListDefPath() string {
	return envOr(EnvAppListDefPath, filepath.Join(ConfigDir(), appListDefName))
}

// VScreenDefPath returns the full path to virtual-screen-def.json.
// Override with VSDPATH.
func VScreenDefPath() string {
	return envOr(EnvVScreenDefPath, filepath.Join(ConfigDir(), vscreenDefName))
}

// LayoutConfigDir returns the directory that holds the layout per-application config
// subtree. Override with LAYOUTPATH.
func LayoutConfigDir() string {
	return envOr(EnvLayoutConfigDir, filepath.Join(AppDir(), layoutSubDir))
}

// LayoutSystemJSONDir returns the directory that holds the layout system/priority
// policy JSON. Override with SYSTEMJSONPATH.
func LayoutSystemJSONDir() string {
	return envOr(EnvLayoutSystemJSONDir, filepath.Join(AppDir(), layoutSubDir))
}

// LifecycleAppDir returns the directory that holds the lifecycle per-application app.json
// files. Override with LIFECYCLEPATH.
func LifecycleAppDir() string {
	return envOr(EnvLifecycleAppDir, AppDir())
}

// RvgpuLockPath returns the lock-file path used to serialize rvgpu index
// allocation. Override with RVGPU_LOCK_FILE.
func RvgpuLockPath() string {
	return envOr(EnvRvgpuLockFile, filepath.Join(RuntimeDir(), rvgpuLockName))
}

// RvgpuIndexPath returns the file path that records the allocated rvgpu index.
// Override with RVGPU_INDEX_FILE.
func RvgpuIndexPath() string {
	return envOr(EnvRvgpuIndexFile, filepath.Join(RuntimeDir(), rvgpuIndexName))
}
