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

package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unified-hmi/internal/config"
	"unified-hmi/internal/lifecycle/rvgpulauncher"
	. "unified-hmi/internal/ulog"
)

type ConsistencyProtocol struct {
	CommType string `json:"type"`
	CommData string `json:"data"`
}

func NcountMaster(
	numWorker int,
	sendNodeChans []chan []byte,
	waitNCountChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	ILog.Printf("(task=%s) Waiting for nCount from (%d) targets...", commTaskCtx.AppName, numWorker)

	connected := 0
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()

LOOP:
	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ELog.Printf("(task=%s) NcountMaster close", commTaskCtx.AppName)
			return

		case <-waitNCountChan:
			connected++
			if connected >= numWorker {
				break LOOP
			}
		case <-t.C:
			ELog.Printf("(task=%s) WatchDog Worker is insufficient(%d < %d)", commTaskCtx.AppName, connected, numWorker)
			ELog.Printf("(task=%s) [CANCEL_REASON] NcountMaster: Worker connection timeout (30 seconds)", commTaskCtx.AppName)
			commTaskCtx.Cancel()
			return
		}
	}

	cp := ConsistencyProtocol{
		CommType: "ncount",
		CommData: "",
	}
	sendMsg, _ := json.Marshal(cp)
	for _, sendNodeChan := range sendNodeChans {
		sendNodeChan <- sendMsg
	}

	ILog.Printf("(task=%s) nCount completed for (%d) targets.", commTaskCtx.AppName, numWorker)
}

func ncountWorker(
	sendLcmChan chan []byte,
	waitNCountChan chan []byte,
	commTaskCtx *CommTaskContext) error {

	rand.Seed(time.Now().UnixNano())

	magicCode := commTaskCtx.AppName + strconv.Itoa(rand.Intn(1024))
	DLog.Println("magicCode: ", magicCode)

	cp := ConsistencyProtocol{
		CommType: "ncount",
		CommData: magicCode,
	}
	sendMsg, _ := json.Marshal(cp)
	sendLcmChan <- sendMsg

	select {
	case recvMsg := <-waitNCountChan:
		if !reflect.DeepEqual(recvMsg, sendMsg) {
			ELog.Printf("(task=%s) [CANCEL_REASON] ncountWorker: Magic code mismatch (expected=%s, received=%s)", commTaskCtx.AppName, sendMsg, recvMsg)
			commTaskCtx.Cancel()
			return errors.New(fmt.Sprintf("magic code mismatch"))
		}
	case <-commTaskCtx.Ctx.Done():
		ELog.Printf("(task=%s) ncountWorker close", commTaskCtx.AppName)
		return errors.New(fmt.Sprintf("other reason"))
	}

	ILog.Printf("(task=%s) nCount completed from Master", commTaskCtx.AppName)
	return nil
}

func NkeepMaster(
	sendNodeChan chan []byte,
	waitNKeepChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	retryInterval := 20 * time.Second
	timeoutInterval := 15 * time.Second

	sendt := time.NewTicker(retryInterval)
	recvt := time.NewTicker(timeoutInterval)
	defer sendt.Stop()
	defer recvt.Stop()

	cp := ConsistencyProtocol{
		CommType: "nkeep",
		CommData: "Check from Master",
	}
	sendMsg, _ := json.Marshal(cp)
	sendNodeChan <- sendMsg

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) NkeepMaster close", commTaskCtx.AppName)
			return

		case <-sendt.C:
			sendNodeChan <- sendMsg
			recvt = time.NewTicker(timeoutInterval)

		case <-recvt.C:
			ELog.Printf("(task=%s) [CANCEL_REASON] NkeepMaster: Health check timeout - No response from worker (15 seconds)", commTaskCtx.AppName)
			commTaskCtx.Cancel()
			return

		case <-waitNKeepChan:
			recvt.Stop()
			sendt = time.NewTicker(retryInterval)
		}
	}
}

func NkeepWorker(
	sendLcmChan chan []byte,
	waitNKeepChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) error {

	defer wg.Done()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown-Host"
	}

	cp := ConsistencyProtocol{
		CommType: "nkeep",
		CommData: "Response from " + commTaskCtx.AppName + " on " + hostname,
	}
	sendMsg, err := json.Marshal(cp)

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) NkeepWorker close", commTaskCtx.AppName)
			return nil

		case <-waitNKeepChan:
			sendLcmChan <- sendMsg
		}
	}

	return nil
}

func processExitHandler(
	pid int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) start ExitHandler for process(pid:%d) \n", commTaskCtx.AppName, pid)
			config.WatchDogKill(pid, true, 60*time.Second)
			return
		}
	}
}

func waitProcess(
	cmd *exec.Cmd,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	pid := cmd.Process.Pid
	DLog.Printf("(task=%s) wait process(pid:%d)\n", commTaskCtx.AppName, pid)
	cmd.Wait()

	ELog.Printf("(task=%s) [CANCEL_REASON] waitProcess: Application process terminated (pid:%d)", commTaskCtx.AppName, pid)
	commTaskCtx.Cancel()
	ILog.Printf("(task=%s) finish process(pid:%d)\n", commTaskCtx.AppName, pid)
}

// Launch targets select the in-process launcher a registered RendererCallback
// has to use instead of executing ExecComm as a child process.
const (
	// LaunchTargetRvgpuRecv runs the receiver side through RvgpuRecvOptions.
	LaunchTargetRvgpuRecv = "rvgpu_recv"
	// LaunchTargetRvgpuSend runs the sender side through RvgpuSendOptions.
	LaunchTargetRvgpuSend = "rvgpu_send"
)

type ExecCommInfo struct {
	ExecComm    string
	ExecCommEnv []string
	IsWaitDeps  bool

	LaunchTarget string

	RvgpuRecvOptions *rvgpulauncher.ReceiverOptions
	RvgpuSendOptions *rvgpulauncher.SenderOptions
}

func TimingWrapper(
	execCommInfo ExecCommInfo,
	sendLcmChan chan []byte,
	waitNCountChan chan []byte,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	var subWg sync.WaitGroup
	var err error

	if execCommInfo.IsWaitDeps {
		err = ncountWorker(sendLcmChan, waitNCountChan, commTaskCtx)
		if err != nil {
			return
		}
	}

	if execCommInfo.LaunchTarget != "" {
		rendererCb := getRendererCallback()
		if rendererCb == nil {
			ELog.Printf("(task=%s) [CANCEL_REASON] no renderer callback registered for launch target %s", commTaskCtx.AppName, execCommInfo.LaunchTarget)
			commTaskCtx.Cancel()
			return
		}

		err = rendererCb(execCommInfo, commTaskCtx)
		if err != nil {
			ELog.Printf("(task=%s) [CANCEL_REASON] rendererCallback failed: %v", commTaskCtx.AppName, err)
			commTaskCtx.Cancel()
			return
		}

		if !execCommInfo.IsWaitDeps {
			err = ncountWorker(sendLcmChan, waitNCountChan, commTaskCtx)
			if err != nil {
				return
			}
		}

		<-commTaskCtx.Ctx.Done()
		commTaskCtx.WaitTeardown()

		if execCommInfo.IsWaitDeps {
			ILog.Printf("(task=%s) timingSyncLaunch finish", commTaskCtx.AppName)
		} else {
			ILog.Printf("(task=%s) timingAsyncLaunch finish", commTaskCtx.AppName)
		}
		return
	}

	cmd := strings.Fields(execCommInfo.ExecComm)
	appCmd := exec.Command(cmd[0], cmd[1:]...)
	appCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	env := make([]string, len(os.Environ()))
	copy(env, os.Environ())
	appCmd.Env = env
	appCmd.Env = append(appCmd.Env, execCommInfo.ExecCommEnv...)
	err = appCmd.Start()
	if err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] execProcess: Failed to start application command: %s (error: %v)", commTaskCtx.AppName, execCommInfo.ExecComm, err)
		commTaskCtx.Cancel()
		return
	}

	subWg.Add(1)
	go processExitHandler(appCmd.Process.Pid, commTaskCtx, &subWg)

	subWg.Add(1)
	go waitProcess(appCmd, commTaskCtx, &subWg)

	if !execCommInfo.IsWaitDeps {
		ncountWorker(sendLcmChan, waitNCountChan, commTaskCtx)
	}

	subWg.Wait()
	if execCommInfo.IsWaitDeps {
		ILog.Printf("(task=%s) timingSyncLaunch finish", commTaskCtx.AppName)
	} else {
		ILog.Printf("(task=%s) timingAsyncLaunch finish", commTaskCtx.AppName)
	}
}
