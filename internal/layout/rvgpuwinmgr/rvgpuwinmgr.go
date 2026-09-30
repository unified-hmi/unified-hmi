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

package rvgpuwinmgr

import (
	"bufio"
	"context"
	"encoding/binary"
	_ "encoding/json"
	_ "errors"
	"fmt"
	"io"
	"net"
	_ "reflect"
	"strconv"
	"sync"
	"time"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/backend"
	. "unified-hmi/internal/ulog"
)

type rvgpuCompositor struct {
	rId             int
	conn            net.Conn
	sendChan        chan string
	waitChan        chan bool
	recvChan        chan []byte
	domainName      string
	justReconnected bool
}

var rvgpuComs = make([]rvgpuCompositor, 0)

func InitRvgpuWinmgr(
	vscrnDef *config.VScrnDef,
	myNodeId int,
	hostName string) bool {

	for _, fwn := range vscrnDef.DistributedWindowSystem.FrameworkNode {
		if fwn.NodeId == myNodeId {
			for _, rdisplay := range vscrnDef.RealDisplays {
				if rdisplay.NodeId == myNodeId {
					for _, vdisplay := range vscrnDef.Def2D.VirtualDisplays {
						if rdisplay.VDisplayId == vdisplay.VDisplayId {
							compositor := rvgpuCompositor{
								rId:        rdisplay.RDisplayId,
								conn:       nil,
								domainName: UHMI_RVGPU_LAYOUT_SOCK + "." + hostName + "_" + strconv.Itoa(rdisplay.RDisplayId),
							}
							rvgpuComs = append(rvgpuComs, compositor)
						}
					}
				}
			}
		}
	}
	if len(rvgpuComs) > 0 {
		DLog.Println(rvgpuComs)
		return true
	}
	return false
}

func retryConnectTarget(sockChan chan net.Conn, stopChan chan struct{}, domainName string) {

	abstract_domain_sock := "@" + domainName
	for {
		conn, err := net.Dial("unix", abstract_domain_sock)
		if err == nil {
			sockChan <- conn
			break
		}
		time.Sleep(10 * time.Millisecond)

		select {
		case <-stopChan:
			return
		default:
		}
	}
}

func connectTarget(domainName string) net.Conn {
	var conn net.Conn

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sockChan := make(chan net.Conn, 1)
	stopChan := make(chan struct{})

	defer func() {
		close(sockChan)
		close(stopChan)
	}()

	go retryConnectTarget(sockChan, stopChan, domainName)

	select {
	case <-ctx.Done():
		ILog.Println("Dial cannot connect rvgpu-compositor: ", domainName)
		return nil
	case conn = <-sockChan:
		ILog.Println("Dial connected to rvgpu-compositor")
	}

	DLog.Printf("connect OK\n")
	return conn
}

func handleConnectTarget(compositor *rvgpuCompositor, wg *sync.WaitGroup) {

	compositor.conn = connectTarget(compositor.domainName)
	if compositor.conn == nil {
		wg.Done()
		return
	}
	compositor.waitChan = make(chan bool, 1)
	compositor.sendChan = make(chan string, 1)
	compositor.recvChan = make(chan []byte, 1)

	defer func() {
		compositor.conn.Close()
		compositor.conn = nil
		close(compositor.waitChan)
		close(compositor.sendChan)
		close(compositor.recvChan)
	}()

	go connReadLoop(compositor.conn, compositor.recvChan)
	wg.Done()

	for {
		select {
		case command := <-compositor.sendChan:
			remoteAddr := compositor.conn.RemoteAddr()
			DLog.Println("sendCommand:", remoteAddr)
			sendCommand(compositor.conn, command)

		case recvMsg := <-compositor.recvChan:
			if recvMsg != nil {
				compositor.waitChan <- true
			} else {
				return
			}
		}
	}
}

func rvgpuMultiConn(compositor *[]rvgpuCompositor) {

	var wg sync.WaitGroup
	for i, comp := range *compositor {
		if comp.conn == nil {
			wg.Add(1)
			go handleConnectTarget(&(*compositor)[i], &wg)
		}
	}
	wg.Wait()
}

func connectTargetOnce(domainName string) net.Conn {
	var conn net.Conn

	abstract_domain_sock := "@" + domainName
	conn, err := net.Dial("unix", abstract_domain_sock)
	if err != nil {
		ELog.Printf("net.Dial: %s \n", err)
		return nil
	}

	DLog.Printf("connect OK\n")
	return conn
}

func handleConnectTargetOnce(compositor *rvgpuCompositor, wg *sync.WaitGroup) {

	compositor.conn = connectTargetOnce(compositor.domainName)
	if compositor.conn == nil {
		wg.Done()
		return
	}

	compositor.waitChan = make(chan bool, 1)
	compositor.sendChan = make(chan string, 1)
	compositor.recvChan = make(chan []byte, 1)
	compositor.justReconnected = true

	defer func() {
		DLog.Println("[2]defunc call")
		compositor.conn.Close()
		compositor.conn = nil
		close(compositor.waitChan)
		close(compositor.sendChan)
		close(compositor.recvChan)
	}()

	go connReadLoop(compositor.conn, compositor.recvChan)
	wg.Done()

	for {
		select {
		case command := <-compositor.sendChan:
			remoteAddr := compositor.conn.RemoteAddr()
			DLog.Println("sendCommand:", remoteAddr)
			sendCommand(compositor.conn, command)

		case recvMsg := <-compositor.recvChan:
			if recvMsg != nil {
				compositor.waitChan <- true
			} else {
				return
			}
		}
	}
}

func rvgpuMultiConnOnce(compositor *[]rvgpuCompositor) {

	var wg sync.WaitGroup
	for i, comp := range *compositor {
		if comp.conn == nil {
			wg.Add(1)
			go handleConnectTargetOnce(&(*compositor)[i], &wg)
		}
	}
	wg.Wait()
}

func (plugin RvgpuPlugin) Start(reqChan chan layoutbackend.LocalCommandReq, respChan chan layoutbackend.LocalCommandReq) {

	rvgpuMultiConn(&rvgpuComs)

	for {
		select {
		case lComReq := <-reqChan:
			rvgpuMultiConnOnce(&rvgpuComs)
			ret := sendRvgpuCompositorJson(&rvgpuComs, lComReq)
			lcr := layoutbackend.LocalCommandReq{}
			lcr.Ret = ret
			respChan <- lcr
			break
		}
	}
}

func sendRvgpuCompositorJson(compositor *[]rvgpuCompositor, lComReq layoutbackend.LocalCommandReq) int {

	msg := ""
	var err error

	DLog.Println("sendRvgpuCompositorJson", lComReq)
	replayReconnected(compositor)

	var rIds = make([]int, 0)
	for i := range *compositor {

		comp := &(*compositor)[i]
		switch lComReq.Command {
		case "initial_vscreen":
			msg, err = genInitialLayoutProtocolJson(lComReq, comp.rId)
		case "add_surface":
			msg, err = genAddSurfaceProtocolJson(lComReq, comp.rId)
		case "modify_surface":
			msg, err = genModifySurfaceProtocolJson(lComReq, comp.rId)
		case "remove_surface":
			msg, err = genRemoveSurfaceProtocolJson(lComReq, comp.rId)
		case "add_layer":
			msg, err = genAddLayerProtocolJson(lComReq, comp.rId)
		case "modify_layer":
			msg, err = genModifyLayerProtocolJson(lComReq, comp.rId)
		case "remove_layer":
			msg, err = genRemoveLayerProtocolJson(lComReq, comp.rId)
		case "local_comm":
			continue
		default:
			fmt.Println("Error lComReq.Command")
			continue
		}

		if msg == "" {
			continue
		}
		if err != nil {
			fmt.Println("Error ProtocolJson")
			continue
		}

		if comp.conn != nil {
			comp.sendChan <- msg
			rIds = append(rIds, comp.rId)
		}
	}

	for i := range *compositor {
		comp := &(*compositor)[i]
		if comp.conn != nil {
			for _, rId := range rIds {
				if rId == comp.rId {
					<-comp.waitChan
				}
			}
		}
	}

	return 0
}

func replayReconnected(compositor *[]rvgpuCompositor) {
	for i := range *compositor {
		comp := &(*compositor)[i]
		if comp.conn == nil || !comp.justReconnected {
			continue
		}
		comp.justReconnected = false
		msg, err := buildInitialLayoutForDisplay(comp.rId)
		if err != nil {
			ELog.Println("replayReconnected build error:", err)
			continue
		}
		if msg == "" {
			ILog.Printf("replayReconnected: no snapshot yet for rId=%d", comp.rId)
			continue
		}
		ILog.Printf("replayReconnected: sending initial_layout to rId=%d (%d bytes)", comp.rId, len(msg))
		comp.sendChan <- msg
		<-comp.waitChan
	}
}

func sendCommand(conn net.Conn, command string) {

	msgLen := uint32(len(command))
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, msgLen)

	n, err := conn.Write(size)
	if err != nil || n == 0 {
		ELog.Printf("Write DATA Size error: %s \n", err)
	}

	n, err = conn.Write([]byte(command))
	if err != nil || n == 0 {
		ELog.Printf("Write error: %s \n", err)
	}
}

func connReadLoop(conn net.Conn, rcvChan chan []byte) {

	cbio := bufio.NewReader(conn)
	for {
		recvSize := 15
		recvBuf := make([]byte, recvSize)
		_, err := io.ReadFull(cbio, recvBuf)
		if err != nil {
			rcvChan <- nil
			return
		}
		rcvChan <- recvBuf
	}
}
