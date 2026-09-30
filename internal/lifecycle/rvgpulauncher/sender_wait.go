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
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
)

const (
	deviceWaitInterval = 500 * time.Millisecond
	deviceWaitTries    = 10

	socketWaitInterval = 100 * time.Millisecond
	socketWaitTries    = 50

	connectionPollInterval = 100 * time.Millisecond
	connectionWaitTimeout  = 10 * time.Second

	socketsPerTarget = 2

	tcpStateEstablished = "01"
)

func virtioDevicePath(cardN int) string {
	return fmt.Sprintf("/dev/dri/rvgpu_virtio%d", cardN)
}

func rvgpuDevicePaths(cardN int) []string {
	return []string{
		virtioDevicePath(cardN),
		fmt.Sprintf("/dev/input/rvgpu_touch%d", cardN),
		fmt.Sprintf("/dev/input/rvgpu_mouse%d", cardN),
		fmt.Sprintf("/dev/input/rvgpu_mouse_abs%d", cardN),
		fmt.Sprintf("/dev/input/rvgpu_keyboard%d", cardN),
	}
}

func waitFileDetection(path string, pid int) error {
	for count := 0; count < deviceWaitTries; count++ {
		if config.CheckProcessAlive(pid) == nil {
			return fmt.Errorf("process %d exited while waiting for %s", pid, path)
		}
		if fileExists(path) {
			return nil
		}
		time.Sleep(deviceWaitInterval)
	}
	return fmt.Errorf("timeout, cannot find %s", path)
}

func waitRvgpuDevices(cardN int, pid int) error {
	paths := rvgpuDevicePaths(cardN)

	var wg sync.WaitGroup
	errs := make(chan error, len(paths))
	for _, path := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			if err := waitFileDetection(path, pid); err != nil {
				errs <- err
			}
		}(path)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		return err
	}
	return nil
}

func waitUnixSocketConnectable(socketPath string, pid int) error {
	for count := 0; count < socketWaitTries; count++ {
		if config.CheckProcessAlive(pid) == nil {
			return fmt.Errorf("process %d exited while waiting for %s", pid, socketPath)
		}

		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			ILog.Println("success connection to wayland server")
			conn.Close()
			return nil
		}
		WLog.Println("failed connect wayland server")

		time.Sleep(socketWaitInterval)
	}
	return fmt.Errorf("timeout, cannot connect wayland server: %s", socketPath)
}

func remoteAddrHex(ip string, port string) (string, error) {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid port number %q: %w", port, err)
	}

	ipv4 := net.ParseIP(ip).To4()
	if ipv4 == nil {
		return "", fmt.Errorf("not an IPv4 address: %q", ip)
	}

	ipParts := make([]string, 4)
	for i, b := range ipv4 {
		ipParts[3-i] = fmt.Sprintf("%02X", b)
	}

	return fmt.Sprintf("%s:%04X", strings.Join(ipParts, ""), portInt), nil
}

func socketInodes(pid int) (map[string]bool, error) {
	fdPath := fmt.Sprintf("/proc/%d/fd/", pid)
	fdEntries, err := ioutil.ReadDir(fdPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", fdPath, err)
	}

	inodes := make(map[string]bool, len(fdEntries))
	for _, fdEntry := range fdEntries {
		link, err := os.Readlink(fdPath + fdEntry.Name())
		if err != nil || !strings.HasPrefix(link, "socket:") {
			continue
		}
		inode := strings.TrimPrefix(link, "socket:[")
		inodes[strings.TrimSuffix(inode, "]")] = true
	}
	return inodes, nil
}

func countEstablishedSockets(r io.Reader, remoteHex string, inodes map[string]bool) int {
	count := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		if fields[2] != remoteHex || fields[3] != tcpStateEstablished {
			continue
		}
		if inodes[fields[9]] {
			DLog.Println("isConnectionEstablished remAddress: ", fields[2], " entryInode: ", fields[9])
			count++
		}
	}
	return count
}

func isConnectionEstablished(pid int, ip string, port string, numSockets int) bool {
	remoteHex, err := remoteAddrHex(ip, port)
	if err != nil {
		ELog.Println("cannot build remote address: ", err)
		return false
	}

	inodes, err := socketInodes(pid)
	if err != nil {
		ELog.Println(err)
		return false
	}

	tcpPath := fmt.Sprintf("/proc/%d/net/tcp", pid)
	tcpFile, err := os.Open(tcpPath)
	if err != nil {
		ELog.Printf("Error opening %s, err %s", tcpPath, err)
		return false
	}
	defer tcpFile.Close()

	count := countEstablishedSockets(tcpFile, remoteHex, inodes)
	if count != numSockets {
		ELog.Printf("haven't ESTABLISHED socket %d < %d yet", count, numSockets)
		return false
	}
	return true
}

func waitForConnectionEstablished(pid int, ip string, port string, numSockets int, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(connectionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timeout reached, connection to %s:%s not established", ip, port)
		case <-ticker.C:
			if isConnectionEstablished(pid, ip, port, numSockets) {
				return nil
			}
		}
	}
}

func waitTargetsConnected(pid int, targets []string) error {
	for _, target := range targets {
		ip, port, err := net.SplitHostPort(target)
		if err != nil {
			return fmt.Errorf("cannot parse target %q: %w", target, err)
		}
		if err := waitForConnectionEstablished(pid, ip, port, socketsPerTarget, connectionWaitTimeout); err != nil {
			return err
		}
	}
	return nil
}
