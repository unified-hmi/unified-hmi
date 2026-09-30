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

package rvgpulauncher

import (
	"fmt"

	"unified-hmi/internal/config"
)

// Options mirrors the command-line options accepted by the
// uhmi-virtio-gpu-wl-send executable. Empty string fields are omitted from the
// rvgpu-proxy argument list, which is how "not set" is expressed.
type SenderOptions struct {
	Capset           string // -c: capset file path
	Scanout          string // -s: scanout in form WxH@X,Y
	Framerate        string // -f: virtual framerate
	AppName          string // -i: rvgpu surface id
	DeveloperOptions *string

	// Targets holds the serverIp:port endpoints of the receiving nodes. When
	// empty, rvgpu-proxy falls back to defaultTarget and no connection
	// establishment is awaited.
	Targets []string

	// TargetApp is the Wayland client to run against the proxy. When empty,
	// Start only brings up the proxy and the caller launches the client.
	TargetApp     string
	TargetAppArgs []string
	// TargetAppEnv holds extra KEY=VALUE entries given to every process Start
	// spawns, applied before the virtio-gpu specific ones.
	TargetAppEnv []string

	// XdgRuntimeDir is where the Wayland socket is created. Empty means
	// $XDG_RUNTIME_DIR, or /tmp when that is unset.
	XdgRuntimeDir string
}

const defaultTarget = "127.0.0.1:55667"

// DefaultSenderOptions returns the option set matching the uhmi-virtio-gpu-wl-send
// command-line defaults, with the environment-driven fields already resolved.
func DefaultSenderOptions() SenderOptions {
	return SenderOptions{
		Scanout:       "1920x1080@0,0",
		XdgRuntimeDir: config.GetEnv("XDG_RUNTIME_DIR", "/tmp"),
	}
}

// BuildSenderProxyArgs converts opts into the rvgpu-proxy argument list.
func BuildSenderProxyArgs(opts SenderOptions) []string {
	args := []string{}
	appendIfSet := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}

	appendIfSet("-c", opts.Capset)
	appendIfSet("-s", opts.Scanout)
	appendIfSet("-f", opts.Framerate)
	appendIfSet("-i", opts.AppName)
	if opts.DeveloperOptions != nil {
		args = append(args, *opts.DeveloperOptions)
	}
	if len(opts.Targets) == 0 {
		args = append(args, "-n", defaultTarget)
	}
	for _, target := range opts.Targets {
		args = append(args, "-n", target)
	}

	return args
}

// ScanoutSize extracts the WxH prefix of a WxH@X,Y scanout specification, which
// is the geometry rvgpu-wlproxy expects.
func ScanoutSize(scanout string) (string, error) {
	var width, height, x, y int
	if _, err := fmt.Sscanf(scanout, "%dx%d@%d,%d", &width, &height, &x, &y); err != nil {
		return "", fmt.Errorf("invalid scanout %q: %w", scanout, err)
	}
	return fmt.Sprintf("%dx%d", width, height), nil
}
