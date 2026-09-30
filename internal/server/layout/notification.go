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

package layoutserver

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

type clientStream struct {
	stream uhmi.UHMIService_SubscribeNotificationsServer
}

type clientWg struct {
	requestWg *sync.WaitGroup
	clientId  string
}

type clientNotification struct {
	RequestId string
	ClientId  string
	Command   string
	RequestWg *sync.WaitGroup
}

type clientRegistry struct {
	mu      sync.Mutex
	streams map[string]*clientStream
}

func newClientRegistry() *clientRegistry {
	return &clientRegistry{streams: make(map[string]*clientStream)}
}

func (r *clientRegistry) add(clientId string, cs *clientStream) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams[clientId] = cs
}

func (r *clientRegistry) remove(clientId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.streams, clientId)
}

func (r *clientRegistry) get(clientId string) (*clientStream, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.streams[clientId]
	return cs, ok
}

type asyncRequestRegistry struct {
	mu   sync.Mutex
	reqs map[string]*clientWg
}

func newAsyncRequestRegistry() *asyncRequestRegistry {
	return &asyncRequestRegistry{reqs: make(map[string]*clientWg)}
}

func (r *asyncRequestRegistry) add(clientId, requestId string) (*clientWg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.reqs[requestId]; exists {
		return nil, errors.New("Cannot use requestId because of occupation. Please specify other requestId, or use LayoutWaitAsyncAnimation")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	cWg := &clientWg{requestWg: &wg, clientId: clientId}
	r.reqs[requestId] = cWg
	return cWg, nil
}

func (r *asyncRequestRegistry) get(requestId string) (*clientWg, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cWg, ok := r.reqs[requestId]
	return cWg, ok
}

func (r *asyncRequestRegistry) remove(requestId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reqs, requestId)
}

func (r *asyncRequestRegistry) removeByWg(target *clientWg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cWg := range r.reqs {
		if cWg == target {
			delete(r.reqs, id)
		}
	}
}

func (r *asyncRequestRegistry) removeByClient(clientId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cWg := range r.reqs {
		if cWg.clientId == clientId {
			delete(r.reqs, id)
		}
	}
}

func (r *asyncRequestRegistry) complete(requestId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cWg, ok := r.reqs[requestId]; ok {
		cWg.requestWg.Done()
		delete(r.reqs, requestId)
	}
}

func (r *asyncRequestRegistry) snapshot() map[string]*clientWg {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*clientWg, len(r.reqs))
	for id, cWg := range r.reqs {
		out[id] = cWg
	}
	return out
}

var (
	clientReg   = newClientRegistry()
	asyncReqReg = newAsyncRequestRegistry()
)

func sendNotification(cN clientNotification) {
	ILog.Printf("sendNotification called! RequestId: %s, ClientId: %s, Command: %s\n", cN.RequestId, cN.ClientId, cN.Command)

	cStream, ok := clientReg.get(cN.ClientId)
	if !ok {
		ILog.Printf("Client stream not found for ClientId: %s", cN.ClientId)
		return
	}
	notif := &uhmi.Notification{
		EventType: cN.Command,
		RequestId: cN.RequestId,
		Status:    "ok",
		Message:   "Command has completed",
	}

	asyncReqReg.complete(cN.RequestId)

	if err := cStream.stream.Send(notif); err != nil {
		ELog.Printf("Error sending notification to ClientId %s: %v", cN.ClientId, err)
	}
}

func (s *Server) SubscribeNotifications(req *uhmi.Empty, stream uhmi.UHMIService_SubscribeNotificationsServer) error {
	serverutil.LogFunc()
	ctx := stream.Context()
	clientId, err := getClientIdFromPeer(ctx)
	if err != nil {
		return err
	}

	cS := &clientStream{stream: stream}
	clientReg.add(clientId, cS)

	<-ctx.Done()
	ILog.Printf("Client %s disconnected: %v", clientId, ctx.Err())
	asyncReqReg.removeByClient(clientId)
	clientReg.remove(clientId)
	return nil
}

func addRequestWg(clientId string, requestId string) error {
	_, err := asyncReqReg.add(clientId, requestId)
	return err
}

func getClientIdFromPeer(ctx context.Context) (string, error) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		ELog.Println("Could not get peer from context")
		return "", grpc.Errorf(grpc.Code(grpc.ErrServerStopped), "Invalid peer address")
	}
	clientId := peer.Addr.String()
	return clientId, nil
}
