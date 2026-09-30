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

	"unified-hmi/internal/config"
	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
)

type receiverRef = nodeRoleRef

var receiverRegistry = newRoleRegistry("receiver")

func addReceiverRef(listenPort int, appName string) bool {
	return receiverRegistry.addRef(listenPort, appName)
}

func setReceiverRefTaskCtx(listenPort int, taskCtx *lifecycle.CommTaskContext) {
	receiverRegistry.setTaskCtx(listenPort, taskCtx)
}

func getReceiverReadyCh(listenPort int) chan struct{} {
	return receiverRegistry.getReadyCh(listenPort)
}

func isReceiverRunning(listenPort int) bool {
	return receiverRegistry.isRunning(listenPort)
}

func removeReceiverRefsForSender(appName string) []int {
	return receiverRegistry.removeRefsForSender(appName)
}

func stopReceiver(listenPort int) {
	receiverRegistry.stop(listenPort)
}

func launchReceiver(receiverJson map[string]interface{}, listenPort int, appName string) {
	launchManagedNode(receiverRegistry, receiverJson, listenPort, appName)
}

func launchDependentReceivers(command []byte, commTaskCtx *lifecycle.CommTaskContext) []int {
	mJson := make(map[string]interface{})
	err := json.Unmarshal(command, &mJson)
	if err != nil {
		return nil
	}

	formatV1, ok := mJson["format_v1"].(map[string]interface{})
	if !ok {
		return nil
	}

	commandType, _ := formatV1["command_type"].(string)
	if formatV1["sender"] == nil {
		return nil
	}

	aip := gVScrnDef.ReadAliasIp()
	lifecycle.ExpandAliasNode(mJson, aip)

	receivers, _ := formatV1["receivers"].([]interface{})

	recvNodes := collectUniqueNodes(receivers)
	cNodes := lifecycle.GetConnectableNode(recvNodes)

	var managedPorts []int

	for _, r := range receivers {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		bp, ok := rMap["backend_params"].(map[string]interface{})
		if !ok {
			continue
		}
		listenPort := int(bp["listen_port"].(float64))

		launcher, ok := rMap["launcher"].(config.LauncherNode)
		if !ok || !lifecycle.IsExistNode(cNodes, launcher) {
			ILog.Printf("(task=%s) receiver port %d: skipped (node not connectable)", commTaskCtx.AppName, listenPort)
			continue
		}

		managedPorts = append(managedPorts, listenPort)
		needLaunch := addReceiverRef(listenPort, commTaskCtx.AppName)

		if needLaunch {
			receiverJson := map[string]interface{}{
				"format_v1": map[string]interface{}{
					"command_type": commandType,
					"appli_name":   commTaskCtx.AppName,
					"receivers":    []interface{}{r},
				},
			}
			go launchReceiver(receiverJson, listenPort, commTaskCtx.AppName)
		}
	}

	return managedPorts
}

func cleanupDependentReceivers(appName string, managedPorts []int) {
	if len(managedPorts) == 0 {
		return
	}
	toStop := removeReceiverRefsForSender(appName)
	for _, port := range toStop {
		stopReceiver(port)
	}
}

func waitForReceiversReady(managedPorts []int, commTaskCtx *lifecycle.CommTaskContext) bool {
	for _, port := range managedPorts {
		readyCh := getReceiverReadyCh(port)
		if readyCh == nil {
			continue
		}
		select {
		case <-readyCh:
			ILog.Printf("(task=%s) receiver port %d: confirmed ready", commTaskCtx.AppName, port)
		case <-commTaskCtx.Ctx.Done():
			ELog.Printf("(task=%s) [CANCEL_REASON] waitForReceiversReady: cancelled while waiting for receiver port %d", commTaskCtx.AppName, port)
			return false
		}
	}
	return true
}
