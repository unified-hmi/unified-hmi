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
	"os"
	"syscall"

	"unified-hmi/internal/config"
)

const cardIndexMax = 65

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func reserveRvgpuIndex() (int, error) {
	indexFile := config.RvgpuIndexPath()
	for cardN := 0; cardN < cardIndexMax; cardN++ {
		if fileExists(virtioDevicePath(cardN)) {
			continue
		}

		if fileExists(indexFile) {
			os.Remove(indexFile)
		}
		file, err := os.OpenFile(indexFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if err != nil {
			return -1, fmt.Errorf("cannot create rvgpu index file %s: %w", indexFile, err)
		}
		defer file.Close()

		if _, err := file.WriteString(fmt.Sprintf("%d\n", cardN)); err != nil {
			return -1, fmt.Errorf("cannot write rvgpu index file %s: %w", indexFile, err)
		}
		return cardN, nil
	}
	return -1, fmt.Errorf("cannot reserve rvgpu index, max card index: %d", cardIndexMax)
}

func lockFileExclusive() (*os.File, error) {
	file, err := os.OpenFile(config.RvgpuLockPath(), os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}

	return file, nil
}

func unlockFile(file *os.File) {
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	file.Close()
}
