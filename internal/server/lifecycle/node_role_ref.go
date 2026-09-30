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

package lifecycleserver

import (
	"encoding/json"
	"fmt"
	"sync"

	"unified-hmi/internal/config"
	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
)

type nodeRoleRef struct {
	listenPort int
	senders    map[string]bool
	taskCtx    *lifecycle.CommTaskContext
	readyCh    chan struct{}
}

type roleRegistry struct {
	mu   sync.Mutex
	role string
	refs map[int]*nodeRoleRef
}

func newRoleRegistry(role string) *roleRegistry {
	return &roleRegistry{role: role, refs: make(map[int]*nodeRoleRef)}
}

func (r *roleRegistry) addRef(listenPort int, appName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	ref, exists := r.refs[listenPort]
	if exists {
		ref.senders[appName] = true
		ILog.Printf("(task=%s) %s port %d: refcount++ -> %d", appName, r.role, listenPort, len(ref.senders))
		return false
	}

	r.refs[listenPort] = &nodeRoleRef{
		listenPort: listenPort,
		senders:    map[string]bool{appName: true},
		readyCh:    make(chan struct{}),
	}
	ILog.Printf("(task=%s) %s port %d: newly registered (refcount=1)", appName, r.role, listenPort)
	return true
}

func (r *roleRegistry) setTaskCtx(listenPort int, taskCtx *lifecycle.CommTaskContext) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ref, exists := r.refs[listenPort]; exists {
		ref.taskCtx = taskCtx
	}
}

func (r *roleRegistry) getReadyCh(listenPort int) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ref, exists := r.refs[listenPort]; exists {
		return ref.readyCh
	}
	return nil
}

func (r *roleRegistry) isRunning(listenPort int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.refs[listenPort]
	return exists
}

func (r *roleRegistry) removeRefsForSender(appName string) []int {
	r.mu.Lock()
	defer r.mu.Unlock()

	var toStop []int
	for port, ref := range r.refs {
		if ref.senders[appName] {
			delete(ref.senders, appName)
			ILog.Printf("(task=%s) %s port %d: refcount-- -> %d", appName, r.role, port, len(ref.senders))
			if len(ref.senders) == 0 {
				toStop = append(toStop, port)
			}
		}
	}
	return toStop
}

func (r *roleRegistry) stop(listenPort int) {
	r.mu.Lock()
	ref, exists := r.refs[listenPort]
	if exists {
		delete(r.refs, listenPort)
	}
	r.mu.Unlock()

	if exists && ref.taskCtx != nil {
		ILog.Printf("%s port %d: refcount=0, stopping", r.role, listenPort)
		ref.taskCtx.Cancel()
	}
}

type readySignaler struct {
	once sync.Once
	ch   chan struct{}
}

func newReadySignaler(ch chan struct{}) *readySignaler {
	return &readySignaler{ch: ch}
}

func (s *readySignaler) signal() {
	s.once.Do(func() {
		if s.ch != nil {
			close(s.ch)
		}
	})
}

func launchManagedNode(r *roleRegistry, nodeJson map[string]interface{}, listenPort int, appName string) {
	taskName := fmt.Sprintf("%s:%d", r.role, listenPort)

	nodeCtx := lifecycle.NewCommTaskCtx(taskName)
	r.setTaskCtx(listenPort, nodeCtx)

	ready := newReadySignaler(r.getReadyCh(listenPort))

	defer func() {
		ready.signal()

		ILog.Printf("(task=%s) %s port %d task ended, cleaning up", appName, r.role, listenPort)

		r.mu.Lock()
		ref, exists := r.refs[listenPort]
		if exists {
			var dependentSenders []string
			for sender := range ref.senders {
				dependentSenders = append(dependentSenders, sender)
			}
			delete(r.refs, listenPort)
			r.mu.Unlock()

			for _, senderName := range dependentSenders {
				ELog.Printf("(task=%s) [CANCEL_REASON] %s port %d stopped, cancelling dependent sender %s",
					taskName, r.role, listenPort, senderName)
				signalCancelCommTaskCtx(senderName)
			}
		} else {
			r.mu.Unlock()
		}
	}()

	newCommand, err := json.Marshal(&nodeJson)
	if err != nil {
		ELog.Printf("(task=%s) %s port %d: failed to marshal command", appName, r.role, listenPort)
		return
	}

	targetAddr := lifecycle.GetDistribNodeAddr(nodeJson)
	if targetAddr == "" {
		ELog.Printf("(task=%s) %s port %d: failed to get target address", appName, r.role, listenPort)
		return
	}

	waitNCountChan := make(chan int, 1)
	nodeNCountChan := make(chan int, 1)
	go func() {
		var signaled bool
		for {
			select {
			case v := <-nodeNCountChan:
				waitNCountChan <- v
				if !signaled {
					ready.signal()
					signaled = true
					ILog.Printf("(task=%s) %s port %d: ncount completed, ready", appName, r.role, listenPort)
				}
			case <-nodeCtx.Ctx.Done():
				return
			}
		}
	}()

	sendNodeChan := make(chan []byte, 1)

	var subWg sync.WaitGroup
	subWg.Add(1)
	go lifecycle.NcountMaster(1, []chan []byte{sendNodeChan}, waitNCountChan, nodeCtx, &subWg)

	subWg.Add(1)
	go lifecycle.HandleNodeConnection(targetAddr, string(newCommand), sendNodeChan, nodeNCountChan, nodeCtx, &subWg)

	subWg.Wait()
}

func collectUniqueNodes(elems []interface{}) []config.LauncherNode {
	var nodes []config.LauncherNode
	for _, e := range elems {
		eMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		node, ok := eMap["launcher"].(config.LauncherNode)
		if !ok {
			continue
		}
		if !lifecycle.IsExistNode(nodes, node) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}
