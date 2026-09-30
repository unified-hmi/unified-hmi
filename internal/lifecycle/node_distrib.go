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
	"net"
	"strconv"
	"sync"
	"time"

	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
)

// IsExistNode reports whether chk is present in dNodes.
func IsExistNode(dNodes []config.LauncherNode, chk config.LauncherNode) bool {
	for _, node := range dNodes {
		if node == chk {
			return true
		}
	}
	return false
}

func chkConnectableNode(node config.LauncherNode, cChan chan bool) {
	addr := node.Ip + ":" + strconv.Itoa(node.Port)

	if !config.ProbeAddr("tcp", addr, 500*time.Millisecond) {
		ILog.Printf("%s not connectable", addr)
		cChan <- false
		return
	}
	cChan <- true
}

// GetConnectableNode probes every node in dNodes concurrently and returns the
// subset that accepted a TCP connection within the dial timeout.
func GetConnectableNode(dNodes []config.LauncherNode) (cNodes []config.LauncherNode) {
	numNodes := len(dNodes)

	cChans := make([]chan bool, numNodes)
	for i := range cChans {
		cChans[i] = make(chan bool, 1)
	}

	for i := 0; i < numNodes; i++ {
		go chkConnectableNode(dNodes[i], cChans[i])
	}

	for i := 0; i < numNodes; i++ {
		connectable := <-cChans[i]
		if connectable {
			cNodes = append(cNodes, dNodes[i])
		}
	}

	DLog.Printf("dNodes %v", dNodes)
	DLog.Printf("cNodes %v", cNodes)

	return
}

// RemoveDisconnectNode filters mJson's sender/receivers/local launcher nodes
// down to those present in cNodes, returning an error if none remain.
func RemoveDisconnectNode(mJson map[string]interface{}, cNodes []config.LauncherNode) error {
	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			chk := sender["launcher"].(config.LauncherNode)
			if !IsExistNode(cNodes, chk) {
				return errors.New("sender is not connectable")
			}
		}

		var Recvs []interface{}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				chk := r.(map[string]interface{})["launcher"].(config.LauncherNode)
				if IsExistNode(cNodes, chk) {
					Recvs = append(Recvs, r)
				}
			}

			if len(Recvs) == 0 {
				return errors.New("not exist connectable receivers")
			}

			mJson["format_v1"].(map[string]interface{})["receivers"] = Recvs
		}

	case "local":
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		chk := local["launcher"].(config.LauncherNode)
		if !IsExistNode(cNodes, chk) {
			return errors.New("local is not connectable")
		}

	default:
		return errors.New("command type err")
	}

	return nil
}

// GetDistribNodeAddr returns the "ip:port" address of the first launcher node
// referenced by mJson (sender, else receivers[0], else local).
func GetDistribNodeAddr(mJson map[string]interface{}) string {
	var node config.LauncherNode

	if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
		sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
		node = sender["launcher"].(config.LauncherNode)

	} else if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
		receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
		node = receivers[0].(map[string]interface{})["launcher"].(config.LauncherNode)

	} else if mJson["format_v1"].(map[string]interface{})["local"] != nil {
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		node = local["launcher"].(config.LauncherNode)

	} else {
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
		return ""
	}

	return node.Ip + ":" + strconv.Itoa(node.Port)
}

// GetDistribNode returns every distinct launcher node referenced by mJson's
// sender/receivers (or local), per command_type.
func GetDistribNode(mJson map[string]interface{}) (dNodes []config.LauncherNode) {
	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			dNodes = append(dNodes, sender["launcher"].(config.LauncherNode))
		}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				chk := r.(map[string]interface{})["launcher"].(config.LauncherNode)
				if !IsExistNode(dNodes, chk) {
					dNodes = append(dNodes, chk)
				}
			}
		}

	case "local":
		if mJson["format_v1"].(map[string]interface{})["local"] != nil {
			local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
			dNodes = append(dNodes, local["launcher"].(config.LauncherNode))
		}

	default:
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
	}

	return
}

// ExpandAliasNode replaces string launcher aliases in mJson's sender/receivers/
// local with the config.LauncherNode they resolve to via alias.
func ExpandAliasNode(mJson map[string]interface{}, alias map[string]config.LauncherNode) {
	var key string

	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			switch sender["launcher"].(type) {
			case string:
				key = sender["launcher"].(string)
				sender["launcher"] = alias[key]
			}
		}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				receiver := r.(map[string]interface{})
				switch receiver["launcher"].(type) {
				case string:
					key = receiver["launcher"].(string)
					receiver["launcher"] = alias[key]
				}
			}
		}
	case "local":
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		switch local["launcher"].(type) {
		case string:
			key = local["launcher"].(string)
			local["launcher"] = alias[key]
		}

	default:
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
	}
}

const workerStopAckTimeout = 90 * time.Second

// HandleNodeConnection connects to addr, sends command as a distribution
// comm, and pumps ncount/nkeep synchronization messages until commTaskCtx is
// done or the connection is lost.
func HandleNodeConnection(
	addr string,
	command string,
	sendNodeChan chan []byte,
	waitNCountChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	conn, err := ConnectTarget(addr)
	if err != nil {
		ELog.Printf("(task=%s) ConnectTarget : %s\n", commTaskCtx.AppName, err)
		return
	}
	ILog.Printf("(task=%s) Dial connected to %s", commTaskCtx.AppName, addr)
	defer conn.Close()

	err = SendCommand(conn, CMD_DistribComm, command)
	if err != nil {
		ELog.Printf("(task=%s) sendCommand : %s\n", commTaskCtx.AppName, err)
		return
	}

	var subWg sync.WaitGroup
	waitNKeepChan := make(chan int, 1)
	subWg.Add(1)
	go NkeepMaster(sendNodeChan, waitNKeepChan, commTaskCtx, &subWg)

	var respNCountMsg []byte
	var cp ConsistencyProtocol
	var workerAlreadyClosed bool

	rcvNodeChan := config.FramePump(commTaskCtx.Ctx, conn, sendNodeChan, config.PumpHandlers{
		MaxFrame: MaxCommandSize,
		TransformSend: func(sendMsg []byte) []byte {
			json.Unmarshal(sendMsg, &cp)
			if cp.CommType == "ncount" {
				return respNCountMsg
			}
			return sendMsg
		},
		OnMessage: func(recvMsg []byte) bool {
			DLog.Printf("recv %s\n", recvMsg)
			json.Unmarshal(recvMsg, &cp)
			switch cp.CommType {
			case "ncount":
				respNCountMsg = recvMsg
				waitNCountChan <- 1
			case "nkeep":
				waitNKeepChan <- 1
			default:
				return false
			}
			return true
		},
		OnClosed: func() {
			workerAlreadyClosed = true
			ELog.Printf("(task=%s) [CANCEL_REASON] HandleNodeConnection: Worker node disconnected unexpectedly (addr: %s)", commTaskCtx.AppName, addr)
			commTaskCtx.Cancel()
		},
		OnSendError: func(err error) {
			ELog.Printf("(task=%s) ERR ConnWriteWithSize : %s\n", commTaskCtx.AppName, err)
		},
	})

	if !workerAlreadyClosed {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if cwErr := tcpConn.CloseWrite(); cwErr != nil {
				ELog.Printf("(task=%s) stop-ack: CloseWrite to %s failed: %v",
					commTaskCtx.AppName, addr, cwErr)
			}
		}

		ackTimer := time.NewTimer(workerStopAckTimeout)
	DRAIN:
		for {
			select {
			case recvMsg := <-rcvNodeChan:
				if recvMsg == nil {
					ILog.Printf("(task=%s) stop-ack: worker %s closed connection",
						commTaskCtx.AppName, addr)
					break DRAIN
				}
			case <-ackTimer.C:
				ELog.Printf("(task=%s) stop-ack: worker %s did not close within %s",
					commTaskCtx.AppName, addr, workerStopAckTimeout)
				break DRAIN
			}
		}
		ackTimer.Stop()
	}

	subWg.Wait()
}

// GetAppInfoFromNode connects to addr, sends comm/data, and pushes the raw
// response (or nil on termination) onto rcvDataCh.
func GetAppInfoFromNode(
	addr string, data string, comm string,
	rcvDataCh chan []byte) {

	DLog.Printf("getAppInfoFromNode targetAddr: %s \n", addr)
	resp, err := config.RequestResponse("tcp", addr, 1*time.Second, 10*time.Millisecond,
		[][]byte{[]byte(comm), []byte(data)}, MaxCommandSize)
	if err != nil {
		DLog.Printf("Termination from launcer : %s \n", err)
		rcvDataCh <- nil
		return
	}
	DLog.Printf("resp from Node: %s \n", resp)
	rcvDataCh <- resp
}
