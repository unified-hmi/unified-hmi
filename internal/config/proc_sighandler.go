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

package config

import (
	"errors"
	"os"
	"syscall"
	"time"

	. "unified-hmi/internal/ulog"
)

// CheckProcessAlive returns the process for pid, or nil when it no longer exists.
func CheckProcessAlive(pid int) *os.Process {
	if pid <= 0 {
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		DLog.Println("cannot find process: ", err)
		return nil
	}
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			return process
		}
		DLog.Printf("PID(%d) does not exist!\n", pid)
		return nil
	}

	return process
}

// KillWhenAlive sends sig to pid when it is still running.
func KillWhenAlive(pid int, sig os.Signal, wait bool) {
	process := CheckProcessAlive(pid)
	if process == nil {
		return
	}
	process.Signal(sig)

	if wait {
		timeout := time.After(time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				if CheckProcessAlive(pid) == nil {
					return
				}
			}
		}
	}
}

// WatchDogKill sends SIGTERM to pid and escalates to SIGKILL after termTimeout.
func WatchDogKill(pid int, wait bool, termTimeout ...time.Duration) {
	timeout := 10 * time.Second
	if len(termTimeout) > 0 && termTimeout[0] > 0 {
		timeout = termTimeout[0]
	}
	process := CheckProcessAlive(pid)
	if process == nil {
		return
	}
	process.Signal(syscall.SIGTERM)

	if wait {
		deadline := time.After(timeout)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline:
				goto sendKill
			case <-ticker.C:
				if CheckProcessAlive(pid) == nil {
					return
				}
			}
		}
	} else {
		time.Sleep(timeout)
	}

sendKill:
	KillWhenAlive(pid, syscall.SIGKILL, wait)
}

func killAllChildren(children []int, wait bool, termTimeout ...time.Duration) {
	for _, pid := range children {
		if wait {
			WatchDogKill(pid, wait, termTimeout...)
			continue
		}
		go WatchDogKill(pid, wait, termTimeout...)
	}
}

// SignalHandler stops tracked child processes after SIGINT or SIGTERM.
func SignalHandler(sigChan <-chan os.Signal, pidChan <-chan int, wait bool, termTimeout ...time.Duration) {
	var children []int
	for {
		select {
		case pid := <-pidChan:
			ILog.Printf("Append child[pid=%d]\n", pid)
			children = append(children, pid)
		case signal := <-sigChan:
			switch signal {
			case syscall.SIGINT, syscall.SIGTERM:
				killAllChildren(children, wait, termTimeout...)
				children = children[:0]
			}
		}
	}
}
