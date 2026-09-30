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
	"errors"
	"fmt"
	"time"

	"unified-hmi/internal/lifecycle"
	"unified-hmi/internal/lifecycle/rvgpulauncher"
	. "unified-hmi/internal/ulog"
)

const rvgpuTermTimeout = 60 * time.Second

func registerPlatformLauncher() {
	lifecycle.SetRendererCallback(func(execCommInfo lifecycle.ExecCommInfo, commTaskCtx *lifecycle.CommTaskContext) error {
		switch execCommInfo.LaunchTarget {
		case lifecycle.LaunchTargetRvgpuRecv:
			return launchRvgpuRecv(execCommInfo, commTaskCtx)
		case lifecycle.LaunchTargetRvgpuSend:
			return launchRvgpuSend(execCommInfo, commTaskCtx)
		default:
			return fmt.Errorf("unsupported launch target: %s", execCommInfo.LaunchTarget)
		}
	})
}

func launchRvgpuRecv(execCommInfo lifecycle.ExecCommInfo, commTaskCtx *lifecycle.CommTaskContext) error {
	if execCommInfo.RvgpuRecvOptions == nil {
		return errors.New("missing rvgpu receiver options")
	}

	opts := *execCommInfo.RvgpuRecvOptions
	opts.Env = execCommInfo.ExecCommEnv

	session, err := rvgpulauncher.StartReceiver(opts)
	if err != nil {
		return err
	}

	go func() {
		session.Wait()
		ELog.Printf("(task=%s) [CANCEL_REASON] rvgpu-renderer terminated", commTaskCtx.AppName)
		commTaskCtx.Cancel()
	}()
	go func() {
		<-commTaskCtx.Ctx.Done()
		session.Terminate(rvgpuTermTimeout)
	}()

	return nil
}
func launchRvgpuSend(execCommInfo lifecycle.ExecCommInfo, commTaskCtx *lifecycle.CommTaskContext) error {
	if execCommInfo.RvgpuSendOptions == nil {
		return errors.New("missing rvgpu sender options")
	}

	opts := rvgpulauncher.ApplySenderEnvironment(*execCommInfo.RvgpuSendOptions, execCommInfo.ExecCommEnv)

	session, err := rvgpulauncher.StartSender(opts)
	if err != nil {
		session.Terminate(rvgpuTermTimeout)
		return err
	}

	go func() {
		<-session.Exited()
		ELog.Printf("(task=%s) [CANCEL_REASON] an rvgpu sender process terminated", commTaskCtx.AppName)
		commTaskCtx.Cancel()
	}()
	commTaskCtx.BeginTeardown()
	go func() {
		defer commTaskCtx.EndTeardown()
		<-commTaskCtx.Ctx.Done()
		session.Terminate(rvgpuTermTimeout)
	}()

	return nil
}
