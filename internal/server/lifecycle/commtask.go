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
	"context"
	"errors"
	"sync"

	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
)

var commTaskCtxMu sync.Mutex

var commTaskCtxMap = make(map[string]*lifecycle.CommTaskContext)

func updateCommTaskCtxMap(appName string) (*lifecycle.CommTaskContext, error) {
	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	if _, exists := commTaskCtxMap[appName]; exists || appName == "" {
		return nil, errors.New("ctxMap already exists: " + appName)
	}
	ctx := lifecycle.NewCommTaskCtx(appName)
	commTaskCtxMap[appName] = ctx

	return ctx, nil
}

func deleteCommTask(appName string) error {
	commTaskCtxMu.Lock()
	ctx, exists := commTaskCtxMap[appName]
	if !exists || appName == "" {
		commTaskCtxMu.Unlock()
		return errors.New("ctxMap does not exist: " + appName)
	}
	delete(commTaskCtxMap, appName)
	commTaskCtxMu.Unlock()

	close(ctx.Finished)
	return nil
}

func cancelCommTaskCtx(waitCtx context.Context, appName string) error {
	commTaskCtxMu.Lock()
	commTaskCtx, exists := commTaskCtxMap[appName]
	commTaskCtxMu.Unlock()

	if !exists {
		return errors.New("no such ctx: " + appName)
	}
	ELog.Printf("(task=%s) [CANCEL_REASON] CancelCommTaskCtx: Explicit stop request received via API", appName)
	commTaskCtx.Cancel()

	select {
	case <-commTaskCtx.Finished:
		return nil
	case <-waitCtx.Done():
		ELog.Printf("(task=%s) cancelCommTaskCtx: wait aborted: %v", appName, waitCtx.Err())
		return waitCtx.Err()
	}
}

func signalCancelCommTaskCtx(appName string) error {
	commTaskCtxMu.Lock()
	commTaskCtx, exists := commTaskCtxMap[appName]
	commTaskCtxMu.Unlock()

	if !exists {
		return errors.New("no such ctx: " + appName)
	}
	ELog.Printf("(task=%s) [CANCEL_REASON] signalCancelCommTaskCtx: dependent cleanup cancel", appName)
	commTaskCtx.Cancel()
	return nil
}

func cancelAllCommTaskCtx(waitCtx context.Context) error {
	commTaskCtxMu.Lock()
	ctxs := make([]*lifecycle.CommTaskContext, 0, len(commTaskCtxMap))
	for appName, commTaskCtx := range commTaskCtxMap {
		ELog.Printf("(task=%s) [CANCEL_REASON] CancelAllCommTaskCtx: Stop all apps request received", appName)
		ctxs = append(ctxs, commTaskCtx)
	}
	commTaskCtxMu.Unlock()

	for _, c := range ctxs {
		c.Cancel()
	}
	for _, c := range ctxs {
		select {
		case <-c.Finished:
		case <-waitCtx.Done():
			ELog.Printf("(task=%s) cancelAllCommTaskCtx: wait aborted: %v", c.AppName, waitCtx.Err())
			return waitCtx.Err()
		}
	}
	return nil
}

func isExistsCommTaskCtx(appName string) bool {
	commTaskCtxMu.Lock()
	_, exists := commTaskCtxMap[appName]
	commTaskCtxMu.Unlock()

	return exists
}

func getRunningAppFromCommTaskCtxMap() []byte {
	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	var appList []byte
	for appName := range commTaskCtxMap {
		if len(appList) > 0 {
			appList = append(appList, ',')
		}
		appList = append(appList, []byte(appName)...)
	}

	return appList
}
