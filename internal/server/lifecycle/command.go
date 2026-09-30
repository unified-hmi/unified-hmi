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
	"errors"
	"strconv"
	"sync"

	"unified-hmi/internal/lifecycle"
	. "unified-hmi/internal/ulog"
)

func normalizeReceiverParams(mJson map[string]interface{}) error {
	formatV1, ok := mJson["format_v1"].(map[string]interface{})
	if !ok {
		return nil
	}

	if receivers, ok := formatV1["receivers"].([]interface{}); ok {
		for _, r := range receivers {
			rMap, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			if err := normalizeDisplayAreaParams(rMap); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeDisplayAreaParams(elem map[string]interface{}) error {
	displayArea, hasDisplayArea := elem["display_area"].(string)
	if !hasDisplayArea || displayArea == "" {
		if launcher, ok := elem["launcher"]; !ok || launcher == nil {
			return errors.New("Either display_area or launcher must be provided")
		}
		return nil
	}

	rdisplay, err := gVScrnDef.GetRealDisplayByDispName(displayArea)
	if err != nil {
		return errors.New("Failed to resolve display_area '" + displayArea + "': " + err.Error())
	}

	hostname, err := gVScrnDef.GetHostNameByNodeId(rdisplay.NodeId)
	if err != nil {
		return errors.New("Failed to get hostname for node_id " + strconv.Itoa(rdisplay.NodeId) + ": " + err.Error())
	}

	elem["launcher"] = hostname

	var backendParams map[string]interface{}
	if bp, ok := elem["backend_params"].(map[string]interface{}); ok {
		backendParams = bp
	} else {
		backendParams = make(map[string]interface{})
		elem["backend_params"] = backendParams
	}

	if _, hasKey := backendParams["scanout_x"]; !hasKey {
		backendParams["scanout_x"] = 0
	}
	if _, hasKey := backendParams["scanout_y"]; !hasKey {
		backendParams["scanout_y"] = 0
	}
	if _, hasKey := backendParams["scanout_w"]; !hasKey {
		backendParams["scanout_w"] = rdisplay.PixelW
	}
	if _, hasKey := backendParams["scanout_h"]; !hasKey {
		backendParams["scanout_h"] = rdisplay.PixelH
	}
	if _, hasKey := backendParams["ivi_surface_id"]; !hasKey {
		backendParams["ivi_surface_id"] = 1991000 + rdisplay.VDisplayId
	}
	if _, hasKey := backendParams["listen_port"]; !hasKey {
		backendParams["listen_port"] = 36000 + rdisplay.VDisplayId
	}
	if _, hasKey := backendParams["sock_domain_name"]; !hasKey {
		backendParams["sock_domain_name"] = hostname + "_" + strconv.Itoa(rdisplay.RDisplayId)
	}
	if _, hasKey := backendParams["rdisplay_id"]; !hasKey {
		backendParams["rdisplay_id"] = rdisplay.RDisplayId
	}

	return nil
}

func splitCommandforEachNode(mJson map[string]interface{}) ([]map[string]interface{}, int) {
	var output []map[string]interface{}

	if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
		senderObj := map[string]interface{}{
			"format_v1": map[string]interface{}{
				"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
				"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
				"sender":       mJson["format_v1"].(map[string]interface{})["sender"],
				"receivers":    mJson["format_v1"].(map[string]interface{})["receivers"],
			},
		}

		output = append(output, senderObj)
	}

	if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
		receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
		for _, receiver := range receivers {

			backendParams := receiver.(map[string]interface{})["backend_params"].(map[string]interface{})
			listenPort := int(backendParams["listen_port"].(float64))

			if isReceiverRunning(listenPort) {
				continue
			}

			receiverObj := map[string]interface{}{
				"format_v1": map[string]interface{}{
					"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
					"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
					"receivers":    []interface{}{receiver},
				},
			}

			output = append(output, receiverObj)
		}
	}

	if mJson["format_v1"].(map[string]interface{})["local"] != nil {
		localObj := map[string]interface{}{
			"format_v1": map[string]interface{}{
				"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
				"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
				"local":        mJson["format_v1"].(map[string]interface{})["local"],
			},
		}
		output = append(output, localObj)
	}

	return output, len(output)
}

func dispatchCommToNodes(command []byte, commTaskCtx *lifecycle.CommTaskContext) {
	mJson := make(map[string]interface{})
	err := json.Unmarshal(command, &mJson)
	if err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] runApp: Failed to parse command JSON (error: %s)", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}

	aip := gVScrnDef.ReadAliasIp()
	lifecycle.ExpandAliasNode(mJson, aip)

	dNodes := lifecycle.GetDistribNode(mJson)
	cNodes := lifecycle.GetConnectableNode(dNodes)

	err = lifecycle.RemoveDisconnectNode(mJson, cNodes)
	if err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] runApp: Failed to remove disconnected nodes (error: %s)", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}
	mJsonList, targetNum := splitCommandforEachNode(mJson)

	waitNCountChan := make(chan int, targetNum)
	sendNodeChans := make([]chan []byte, targetNum)
	for i := range sendNodeChans {
		sendNodeChans[i] = make(chan []byte, 1)
	}

	var subWg sync.WaitGroup
	subWg.Add(1)
	go lifecycle.NcountMaster(targetNum, sendNodeChans, waitNCountChan, commTaskCtx, &subWg)

	for i, mJson := range mJsonList {
		targetAddr := lifecycle.GetDistribNodeAddr(mJson)
		newCommand, _ := json.Marshal(&mJson)
		subWg.Add(1)
		go lifecycle.HandleNodeConnection(targetAddr, string(newCommand), sendNodeChans[i], waitNCountChan, commTaskCtx, &subWg)
	}

	subWg.Wait()
}

func dispatchAppWithReceivers(command []byte, commTaskCtx *lifecycle.CommTaskContext) {
	mJson := make(map[string]interface{})
	if err := json.Unmarshal(command, &mJson); err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] dispatchAppWithReceivers: Failed to parse command JSON: %s", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}

	if err := normalizeReceiverParams(mJson); err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] dispatchAppWithReceivers: Failed to normalize display_area params: %s", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}

	normalizedCmd, err := json.Marshal(mJson)
	if err != nil {
		ELog.Printf("(task=%s) [CANCEL_REASON] dispatchAppWithReceivers: Failed to marshal normalized command: %s", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}
	DLog.Printf("(task=%s) dispatchAppWithReceivers: normalizedCmd=%s", commTaskCtx.AppName, normalizedCmd)

	managedPorts := launchDependentReceivers(normalizedCmd, commTaskCtx)
	defer cleanupDependentReceivers(commTaskCtx.AppName, managedPorts)

	if len(managedPorts) > 0 {
		if !waitForReceiversReady(managedPorts, commTaskCtx) {
			return
		}
		ILog.Printf("(task=%s) all %d dependent receivers ready, proceeding with dispatch", commTaskCtx.AppName, len(managedPorts))
	}

	dispatchCommToNodes(normalizedCmd, commTaskCtx)
}
