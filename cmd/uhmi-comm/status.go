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

package main

import (
	"fmt"
	"os"
	"strings"

	"unified-hmi/internal/config"
)

type availableAppDisplay struct {
	name   string
	source string
}

func formatAvailableAppList(info string) []availableAppDisplay {
	const unknownSource = "unknown"
	sources := []string{"app.json", "app-list-def.json"}
	entries := strings.Split(info, ",")
	apps := make([]availableAppDisplay, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		app := availableAppDisplay{name: entry, source: unknownSource}
		for _, source := range sources {
			suffix := "(" + source + ")"
			if strings.HasSuffix(entry, suffix) {
				app.name = strings.TrimSpace(strings.TrimSuffix(entry, suffix))
				app.source = source
				break
			}
		}
		if app.name != "" {
			apps = append(apps, app)
		}
	}
	return apps
}

// statusToExitCode maps the server's per-command Status vocabulary to an
// exit-code style result (0 = success, 1 = failure). uhmi-master-node
// currently answers with three different status dialects depending on the
// handler group:
//
//	A: "success" / "error"                      (uhmi launch/mirror/priority handlers)
//	B: "Success" / "Error" / "Finish" / "Busy"  (config.STAT_Exec*, lifecycle handlers)
//	C: "<message> successfully"                 (layout/animation handlers)
//
// Dialect C failures arrive as non-nil gRPC errors and never reach this
// function. Anything not recognized as success fails safe to 1.
func statusToExitCode(status string) int {
	switch {
	case strings.EqualFold(status, "success"): // dialects A and B ("Success")
		return 0
	case strings.EqualFold(status, config.STAT_ExecFin): // dialect B ("Finish")
		return 0
	case strings.HasSuffix(strings.ToLower(status), "successfully"): // dialect C
		return 0
	default:
		return 1
	}
}

// reportStatus returns the exit-code style result for status. On failure it
// also prints an explicit "<cmd> failed" line to stderr, because dialect
// A/B failures otherwise surface only as a bare status string while the
// process exit code used to stay zero.
func reportStatus(cmd, status string) int {
	if statusToExitCode(status) == 0 {
		return 0
	}
	if !isKnownFailureStatus(status) {
		fmt.Fprintf(os.Stderr, "%s: unrecognized status %q (treated as failure)\n", cmd, status)
	}
	fmt.Fprintf(os.Stderr, "%s failed (status=%s)\n", cmd, status)
	return 1
}

// isKnownFailureStatus reports whether status is a known failure token from
// one of the three dialects, as opposed to an unrecognized value.
func isKnownFailureStatus(status string) bool {
	switch {
	case strings.EqualFold(status, "error"): // dialects A and B ("Error")
		return true
	case strings.EqualFold(status, config.STAT_ExecBusy): // dialect B ("Busy")
		return true
	case strings.HasPrefix(status, "Failed to "): // dialect C
		return true
	default:
		return false
	}
}

// resultLabel renders handleCommand's return value for the final result line:
// 0 = success, 1 = the server reported a failure status, anything else
// (notably -1) = no server status was ever reached (bad args, local I/O, or
// the RPC call itself failed), regardless of which side is at fault.
func resultLabel(val int) string {
	switch val {
	case 0:
		return "success"
	case 1:
		return "error"
	default:
		return "request_error"
	}
}
