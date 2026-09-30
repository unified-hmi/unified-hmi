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
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
)

const rendererCommand = "rvgpu-renderer"

// ReceiverSession tracks a running rvgpu-renderer process.
type ReceiverSession struct {
	cmd     *exec.Cmd
	exited  chan struct{}
	waitErr error
}

// Pid returns the rvgpu-renderer process id.
func (s *ReceiverSession) Pid() int {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Wait blocks until rvgpu-renderer exits and reports its exit error.
// It is safe to call from several goroutines and more than once.
func (s *ReceiverSession) Wait() error {
	<-s.exited
	return s.waitErr
}

// Terminate asks rvgpu-renderer to stop, escalating to SIGKILL after
// termTimeout, and blocks until the process is gone.
func (s *ReceiverSession) Terminate(termTimeout time.Duration) {
	pid := s.Pid()
	if pid == 0 {
		return
	}
	config.WatchDogKill(pid, true, termTimeout)
	<-s.exited
}

// WaylandServerPath returns the Wayland socket rvgpu-renderer will display on.
// Entries in env take precedence over the process environment.
func WaylandServerPath(env []string) string {
	xdgRuntimeDir, ok := EnvValue(env, "XDG_RUNTIME_DIR")
	if !ok {
		xdgRuntimeDir = config.GetEnv("XDG_RUNTIME_DIR", "/run/user/1000")
	}
	wlDisplay, ok := EnvValue(env, "WAYLAND_DISPLAY")
	if !ok {
		wlDisplay = config.GetEnv("WAYLAND_DISPLAY", "wayland-0")
	}
	return xdgRuntimeDir + "/" + wlDisplay
}

// CheckWaylandServer verifies that the Wayland compositor socket accepts
// connections before rvgpu-renderer is started against it.
func CheckWaylandServer(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("cannot connect wayland server %s: %w", socketPath, err)
	}
	conn.Close()
	return nil
}

// StartReceiver launches rvgpu-renderer with opts.
func StartReceiver(opts ReceiverOptions) (*ReceiverSession, error) {
	wlServerPath := WaylandServerPath(opts.Env)
	if err := CheckWaylandServer(wlServerPath); err != nil {
		return nil, err
	}

	cmd := exec.Command(rendererCommand, BuildReceiverArgs(opts)...)
	cmd.Env = append(os.Environ(), opts.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start %s: %w", rendererCommand, err)
	}

	session := &ReceiverSession{cmd: cmd, exited: make(chan struct{})}
	go func() {
		pid := session.Pid()
		DLog.Printf("wait process(app=%s pid:%d)\n", rendererCommand, pid)
		session.waitErr = cmd.Wait()
		ILog.Printf("finish process(app=%s pid:%d)\n", rendererCommand, pid)
		close(session.exited)
	}()

	return session, nil
}
