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

// Package rvgpulauncher runs the sending and receiving sides of remote virtio-gpu
// session, together with rvgpu-wlproxy and the Wayland client that renders into
// it. It backs both the uhmi-virtio-gpu-wl-send executable and the in-process
// launch path used by uhmi-worker-node.
package rvgpulauncher

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
)

const (
	proxyCommand   = "rvgpu-proxy"
	wlProxyCommand = "rvgpu-wlproxy"
)

type child struct {
	name   string
	cmd    *exec.Cmd
	exited chan struct{}
}

func (c *child) pid() int {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func (c *child) terminate(termTimeout time.Duration) {
	if c == nil {
		return
	}

	select {
	case <-c.exited:
		return
	default:
	}

	if pid := c.pid(); pid > 0 {
		config.WatchDogKill(pid, true, termTimeout)
	}
	<-c.exited
}

// SenderSession owns the processes started for one sending-side virtio-gpu session.
type SenderSession struct {
	// WlSocketName is the WAYLAND_DISPLAY value clients of this session must use.
	WlSocketName string
	// XdgRuntimeDir is the directory holding WlSocketName.
	XdgRuntimeDir string

	proxy   *child
	wlProxy *child
	app     *child

	lockMu sync.Mutex
	lock   *os.File

	exitedOnce sync.Once
	anyExited  chan struct{}

	terminateOnce sync.Once
	terminated    chan struct{}
}

func newSenderSession(xdgRuntimeDir string) *SenderSession {
	return &SenderSession{
		XdgRuntimeDir: xdgRuntimeDir,
		anyExited:     make(chan struct{}),
		terminated:    make(chan struct{}),
	}
}

// Exited is closed as soon as any child of the session terminates.
func (s *SenderSession) Exited() <-chan struct{} { return s.anyExited }

// Wait blocks until every child of the session has terminated.
func (s *SenderSession) Wait() {
	for _, c := range []*child{s.proxy, s.wlProxy, s.app} {
		if c != nil {
			<-c.exited
		}
	}
}

// Terminate stops every child in shutdown order, escalating to SIGKILL after
// termTimeout, and blocks until they are gone.
func (s *SenderSession) Terminate(termTimeout time.Duration) {
	s.terminateOnce.Do(func() {
		for _, child := range []*child{s.app, s.wlProxy, s.proxy} {
			child.terminate(termTimeout)
		}
		s.Close()
		close(s.terminated)
	})
	<-s.terminated
}

// Close releases the rvgpu index lock when the session still holds it.
func (s *SenderSession) Close() {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.lock != nil {
		unlockFile(s.lock)
		s.lock = nil
	}
}

func (s *SenderSession) acquireLock() error {
	lock, err := lockFileExclusive()
	if err != nil {
		return fmt.Errorf("unable to lock rvgpu index file: %w", err)
	}
	s.lockMu.Lock()
	s.lock = lock
	s.lockMu.Unlock()
	return nil
}

func (s *SenderSession) startChild(name string, cmd *exec.Cmd) (*child, error) {
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start %s: %w", name, err)
	}

	c := &child{name: name, cmd: cmd, exited: make(chan struct{})}
	go func() {
		DLog.Printf("wait process(app=%s pid:%d)\n", c.name, c.pid())
		cmd.Wait()
		ILog.Printf("finish process(app=%s pid:%d)\n", c.name, c.pid())
		close(c.exited)
		s.exitedOnce.Do(func() { close(s.anyExited) })
	}()
	return c, nil
}

func newCommand(name string, args []string, env []string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// StartSender brings up the sending side described by opts. The returned session is
// never nil so that the caller can shut down partially started children even
// when Start reports an error.
func StartSender(opts SenderOptions) (*SenderSession, error) {
	xdgRuntimeDir := opts.XdgRuntimeDir
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = DefaultSenderOptions().XdgRuntimeDir
	}
	session := newSenderSession(xdgRuntimeDir)

	size := ""
	if opts.Scanout != "" {
		var err error
		if size, err = ScanoutSize(opts.Scanout); err != nil {
			return session, err
		}
	}

	proxyArgs := BuildSenderProxyArgs(opts)

	err := session.startProxyWithWlProxy(proxyArgs, size, opts.Targets, opts.TargetAppEnv)
	session.Close()
	if err != nil {
		return session, err
	}

	if opts.TargetApp != "" {
		if err := session.startTargetApp(opts); err != nil {
			return session, err
		}
	}

	return session, nil
}

func (s *SenderSession) startProxyWithWlProxy(proxyArgs []string, size string, targets []string, appEnv []string) error {
	if err := s.acquireLock(); err != nil {
		return err
	}

	cardN, err := reserveRvgpuIndex()
	if err != nil {
		return err
	}

	s.WlSocketName = fmt.Sprintf("rvgpu-wayland-%d", cardN)
	wlServerPath := s.XdgRuntimeDir + "/" + s.WlSocketName

	if s.proxy, err = s.startChild(proxyCommand, newCommand(proxyCommand, proxyArgs, appEnv)); err != nil {
		return err
	}

	if err := waitRvgpuDevices(cardN, s.proxy.pid()); err != nil {
		return fmt.Errorf("cannot find all rvgpu card and input devices: %w", err)
	}
	s.Close()

	wlProxyEnv := append([]string(nil), appEnv...)
	wlProxyEnv = append(wlProxyEnv,
		fmt.Sprintf("EGLWINSYS_DRM_DEV_NAME=/dev/dri/rvgpu_virtio%d", cardN),
		fmt.Sprintf("EGLWINSYS_DRM_TOUCH_DEV=/dev/input/rvgpu_touch%d", cardN),
		fmt.Sprintf("EGLWINSYS_DRM_MOUSE_DEV=/dev/input/rvgpu_mouse%d", cardN),
		fmt.Sprintf("EGLWINSYS_DRM_MOUSEABS_DEV=/dev/input/rvgpu_mouse_abs%d", cardN),
		fmt.Sprintf("EGLWINSYS_DRM_KEYBOARD_DEV=/dev/input/rvgpu_keyboard%d", cardN),
		"__GLX_VENDOR_LIBRARY_NAME=mesa",
		"MESA_LOADER_DRIVER_OVERRIDE=virtio_gpu",
		"XDG_RUNTIME_DIR="+s.XdgRuntimeDir,
	)
	wlProxyArgs := []string{"-s", size, "-S", s.WlSocketName, "-f"}

	if s.wlProxy, err = s.startChild(wlProxyCommand, newCommand(wlProxyCommand, wlProxyArgs, wlProxyEnv)); err != nil {
		return err
	}

	if err := waitUnixSocketConnectable(wlServerPath, s.wlProxy.pid()); err != nil {
		return err
	}

	return waitTargetsConnected(s.proxy.pid(), targets)
}

func (s *SenderSession) startTargetApp(opts SenderOptions) error {
	env := append([]string(nil), opts.TargetAppEnv...)
	env = append(env,
		"__GLX_VENDOR_LIBRARY_NAME=mesa",
		"MESA_LOADER_DRIVER_OVERRIDE=virtio_gpu",
		"XDG_RUNTIME_DIR="+s.XdgRuntimeDir,
		"WAYLAND_DISPLAY="+s.WlSocketName,
	)

	ILog.Printf("start target app: %q args=%v", opts.TargetApp, opts.TargetAppArgs)

	var err error
	s.app, err = s.startChild(opts.TargetApp, newCommand(opts.TargetApp, opts.TargetAppArgs, env))
	return err
}
