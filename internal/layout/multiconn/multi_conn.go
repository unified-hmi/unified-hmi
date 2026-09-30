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

package layoutmulticonn

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/vscreen"
	. "unified-hmi/internal/ulog"
)

var MulCon *MultiConnector

var Mutex struct {
	sync.Mutex
}

// ResyncProvider returns an initial_vscreen JSON command that represents the
// full intended scene state cached at the master, suitable for pushing to a
// worker that just (re)connected. Returning ("", nil) means "nothing to
// replay" (e.g. cache empty at startup).
type ResyncProvider func(nodeId int) (string, error)

var resyncProviderV atomic.Value

// SetResyncProvider registers a callback used to obtain the initial_vscreen
// snapshot pushed to a worker on (re)connect. Safe to call once at startup.
func SetResyncProvider(p ResyncProvider) {
	if p == nil {
		return
	}
	resyncProviderV.Store(p)
}

func loadResyncProvider() ResyncProvider {
	v := resyncProviderV.Load()
	if v == nil {
		return nil
	}
	return v.(ResyncProvider)
}

type TargetNodeAddr struct {
	NodeId     int
	TargetAddr string
}

type MultiConnector struct {
	targetNodeAddrs []TargetNodeAddr
	sendChans       []chan string
	respChans       []chan CommandResponse
	force           bool
}

type CommandResponse struct {
	Type   string
	Result int
}

func newMultiConn(vscrnDef *config.VScrnDef, force bool) (*MultiConnector, error) {

	dNodes, err := vscrnDef.GetDistribNodes()
	if err != nil {
		return nil, err
	}

	targetNum := 0
	var targets []TargetNodeAddr
	for _, d := range dNodes {
		targetAddr := d.Ip + ":" + strconv.Itoa(d.LayoutPort)
		targets = append(targets, TargetNodeAddr{
			NodeId:     d.NodeId,
			TargetAddr: targetAddr,
		})
		targetNum++
	}

	if targetNum == 0 {
		return nil, errors.New("targetNode is not set correctly. Please check your virtual-screen-def.json file")
	}

	if MulCon != nil && reflect.DeepEqual(MulCon.targetNodeAddrs, targets) {
		WLog.Println("MulCon has already initialized")
		return nil, nil
	}

	sendChans := make([]chan string, len(targets))
	respChans := make([]chan CommandResponse, len(targets))
	for i := range targets {
		sendChans[i] = nil
		respChans[i] = nil
	}

	var MulCon *MultiConnector
	MulCon = &MultiConnector{
		targetNodeAddrs: targets,
		sendChans:       sendChans,
		respChans:       respChans,
		force:           force,
	}
	return MulCon, nil
}

func connectTarget(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err == nil {
		ILog.Println("Dial connected to ", addr)
		return conn, nil
	}

	if timeout > 0 {
		conn, err = config.DialWithTimeout("tcp", addr, timeout*time.Second, 10*time.Millisecond)
		if err != nil {
			return nil, errors.New("Dial cannot connect to master")
		}
		ILog.Println("Retry Dial connected to ", addr)
		return conn, nil
	}

	return nil, errors.New("Dial cannot connect to master")
}

func sendCommand(conn net.Conn, command string) ([]byte, uint32, error) {

	DLog.Println("Write JSON size:", len(command))
	DLog.Println("Write JSON data:", command)
	if err := config.WriteFrame(conn, []byte(command)); err != nil {
		ELog.Printf("Write error: %s \n", err)
		return nil, 0, err
	}

	respBuf, err := config.ReadFrame(conn, 0)
	if err != nil {
		ELog.Printf("Read error: %s \n", err)
		return nil, 0, err
	}
	return respBuf, uint32(len(respBuf)), nil
}

func isConnDropError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.EPIPE)
}

func handleConnectTarget(ums *MultiConnector, chanId int, targetNodeAddr TargetNodeAddr, sendChan chan string, respChan chan CommandResponse, wg *sync.WaitGroup) {

	var err error
	conn, err := connectTarget(targetNodeAddr.TargetAddr, 0)
	if err != nil {
		WLog.Println("Failed connect target: ", targetNodeAddr.TargetAddr, " err: ", err)
		wg.Done()
		return
	}
	defer conn.Close()

	Mutex.Lock()
	ums.sendChans[chanId] = sendChan
	ums.respChans[chanId] = respChan
	Mutex.Unlock()

	resyncNode(conn, targetNodeAddr)

	wg.Done()
	for {
		select {
		case command := <-sendChan:
			jsonCommand, err := layoutvscreen.ApplyAndGenCommand(command, targetNodeAddr.NodeId)
			if err != nil {
				ELog.Printf("Apply and Generate command Fail: %s \n", err)
				respChan <- CommandResponse{Type: "result", Result: -1}
				continue
			}
			respBuf, respSize, err := sendCommand(conn, jsonCommand)
			if err != nil && isConnDropError(err) {
				WLog.Println("Connection closed, Retrying connect to ", targetNodeAddr.TargetAddr)
				conn.Close()
				var reErr error
				conn, reErr = connectTarget(targetNodeAddr.TargetAddr, 1)
				if reErr != nil {
					WLog.Println("Reconnection failed for ", targetNodeAddr.TargetAddr)
					Mutex.Lock()
					ums.sendChans[chanId] = nil
					ums.respChans[chanId] = nil
					Mutex.Unlock()
					respChan <- CommandResponse{Type: "result", Result: -1}
					return
				}
				ILog.Println("Successfully reconnected to ", targetNodeAddr.TargetAddr)
				resyncNode(conn, targetNodeAddr)
				respBuf, respSize, err = sendCommand(conn, jsonCommand)
			}

			var ucr CommandResponse
			if err != nil {
				ELog.Printf("Send command Fail: %s \n", err)
				ucr = CommandResponse{Type: "result", Result: -1}
			} else {
				if err = json.Unmarshal([]byte(string(respBuf[:respSize])), &ucr); err != nil {
					ELog.Printf("Unmarshal json command error: %s \n", err)
					ucr = CommandResponse{Type: "result", Result: -1}
				}
			}

			respChan <- ucr
		}
	}
}

func resyncNode(conn net.Conn, targetNodeAddr TargetNodeAddr) {
	provider := loadResyncProvider()
	if provider == nil {
		return
	}
	cmd, err := provider(targetNodeAddr.NodeId)
	if err != nil {
		ELog.Printf("resync build failed for node %d: %v", targetNodeAddr.NodeId, err)
		return
	}
	if cmd == "" {
		return
	}
	jsonCmd, err := layoutvscreen.ApplyAndGenCommand(cmd, targetNodeAddr.NodeId)
	if err != nil {
		ELog.Printf("resync ApplyAndGenCommand failed for node %d: %v", targetNodeAddr.NodeId, err)
		return
	}
	if _, _, err := sendCommand(conn, jsonCmd); err != nil {
		WLog.Printf("resync sendCommand failed for node %d (%s): %v", targetNodeAddr.NodeId, targetNodeAddr.TargetAddr, err)
		return
	}
	ILog.Printf("resync sent initial_vscreen to node %d (%s)", targetNodeAddr.NodeId, targetNodeAddr.TargetAddr)
}

func (ums *MultiConnector) countConnection() int {
	connectNum := 0
	for chanId := range ums.targetNodeAddrs {
		if ums.sendChans[chanId] == nil {
			continue
		}
		if ums.respChans[chanId] == nil {
			continue
		}
		connectNum++
	}

	return connectNum
}

func (ums *MultiConnector) handleConnectTargets() {

	var wg sync.WaitGroup
	for chanId, targetNodeAddr := range ums.targetNodeAddrs {
		if ums.sendChans[chanId] == nil || ums.respChans[chanId] == nil {
			wg.Add(1)
			sendChan := make(chan string, 1)
			respChan := make(chan CommandResponse, 1)
			go handleConnectTarget(ums, chanId, targetNodeAddr, sendChan, respChan, &wg)
		} else {
			WLog.Println("targetNodeAddr ", targetNodeAddr, " has already connected")
		}
	}
	wg.Wait()
}

func waitResponse(waitTime time.Duration, respChan chan CommandResponse, targetAddr string, wg *sync.WaitGroup, resp **CommandResponse) {

	t := time.NewTicker(waitTime * time.Second)
	defer t.Stop()
	defer wg.Done()

	select {
	case ucr := <-respChan:
		*resp = &ucr
		break
	case <-t.C:
		timeoutResp := CommandResponse{
			Type:   "result",
			Result: -1,
		}
		*resp = &timeoutResp
		ELog.Printf("Command response watchdog was timeout. target: %s", targetAddr)
		break
	}
}

func (ums *MultiConnector) sendCommand(command string) CommandResponse {
	Mutex.Lock()
	var wg sync.WaitGroup
	resps := make([]*CommandResponse, len(ums.sendChans))
	for chanId, sendChan := range ums.sendChans {
		if sendChan != nil && ums.respChans[chanId] != nil {
			wg.Add(1)
			sendChan <- command
			go waitResponse(1, ums.respChans[chanId], ums.targetNodeAddrs[chanId].TargetAddr, &wg, &resps[chanId])
		}
	}
	wg.Wait()

	ret := mergeResponses(resps)
	Mutex.Unlock()

	return ret
}

func (ums *MultiConnector) SendLayoutCommand(command string) error {
	connectNum := ums.countConnection()
	if connectNum < len(ums.targetNodeAddrs) {
		ums.handleConnectTargets()
		connectNum = ums.countConnection()
		if connectNum == 0 {
			return errors.New("All targets cannot connect master")
		}

		if !ums.force {
			if connectNum < len(ums.targetNodeAddrs) {
				return errors.New(fmt.Sprintf("Some targets cannot connect master (%d < %d)", connectNum, len(ums.targetNodeAddrs)))
			}
		}
	}

	ucr := ums.sendCommand(command)
	if ucr.Type == "result" {
		ret := ucr.Result
		if ret != 0 {
			return errors.New("SendLayoutCommand Failed")
		}
	} else {
		return errors.New("result format type miss matched")
	}

	return nil
}

func mergeResponses(resps []*CommandResponse) CommandResponse {
	var ret CommandResponse
	for _, resp := range resps {
		if resp != nil {
			ret.Type = resp.Type
			switch ret.Type {
			case "result":
				ret.Result = ret.Result | resp.Result
				break
			}
		}
	}
	return ret

}

func ConnectionInit(vscrnDef *config.VScrnDef, force bool) error {
	var err error
	MulCon, err = newMultiConn(vscrnDef, force)
	if err != nil {
		return err
	}

	MulCon.handleConnectTargets()
	return nil
}
