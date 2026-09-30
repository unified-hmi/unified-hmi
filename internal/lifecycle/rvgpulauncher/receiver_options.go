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

import "strings"

// Options mirrors the command-line options accepted by the uhmi-virtio-gpu-recv
// executable. Empty string fields are omitted from the rvgpu-renderer argument
// list, which is how "not set" is expressed for the optional switches.
type ReceiverOptions struct {
	Color            string // -B: initial screen color in RGBA
	Box              string // -b: scanout box, WxH@X,Y
	SfcId            string // -i: scanout window (surface) id
	Card             string // -g: GBM mode card
	Domain           string // -d: unix socket domain name
	Seat             string // -S: input seat in GBM mode
	Output           string // -f: output id for Wayland fullscreen mode
	Port             string // -p: listening port
	Fps              string // -V: vsync framerate, only used when Vsync is set
	DumpFile         string // -F: FPS / frame time measurement dump file
	DeveloperOptions *string
	Translucent      bool // -a
	Vsync            bool // -v
	LayoutMode       bool // -l
	LayoutSync       bool // -L

	// Env holds extra KEY=VALUE entries for the rvgpu-renderer process. Its
	// XDG_RUNTIME_DIR and WAYLAND_DISPLAY entries also select the Wayland socket
	// the renderer is checked against.
	Env []string
}

// EnvValue returns the value of key in a KEY=VALUE list.
func EnvValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

// DefaultReceiverOptions returns the option set matching the uhmi-virtio-gpu-recv
// command-line defaults.
func DefaultReceiverOptions() ReceiverOptions {
	return ReceiverOptions{
		Color:       "0x33333333",
		Box:         "1920x1080@0,0",
		SfcId:       "9000",
		Domain:      "rvgpu-compositor-0",
		Port:        "55667",
		Fps:         "60",
		Translucent: true,
		Vsync:       false,
		LayoutMode:  true,
	}
}

// BuildReceiverArgs converts opts into the rvgpu-renderer argument list.
func BuildReceiverArgs(opts ReceiverOptions) []string {
	args := []string{}
	appendIfSet := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}

	appendIfSet("-B", opts.Color)
	appendIfSet("-b", opts.Box)
	appendIfSet("-i", opts.SfcId)
	appendIfSet("-g", opts.Card)
	appendIfSet("-d", opts.Domain)
	appendIfSet("-S", opts.Seat)
	appendIfSet("-f", opts.Output)
	appendIfSet("-p", opts.Port)
	appendIfSet("-F", opts.DumpFile)
	if opts.Translucent {
		args = append(args, "-a")
	}
	if opts.Vsync {
		args = append(args, "-v")
		appendIfSet("-V", opts.Fps)
	}
	if opts.LayoutMode {
		args = append(args, "-l")
	}
	if opts.LayoutSync {
		args = append(args, "-L")
	}
	if opts.DeveloperOptions != nil {
		args = append(args, *opts.DeveloperOptions)
	}

	return args
}
