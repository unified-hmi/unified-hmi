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

	layoutanimation "unified-hmi/internal/layout/animation"
	layoutcore "unified-hmi/internal/layout/core"
	layoutparams "unified-hmi/internal/layout/params"
	layoutvscreen "unified-hmi/internal/layout/vscreen"
	"unified-hmi/internal/server/util"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func (s *Server) StartSystemAnimation(ctx context.Context, req *uhmi.StartSystemAnimationRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	areaName := req.GetAreaName()
	patternName := req.GetPatternName()

	as, err := layoutparams.GetAnimationSetting(appName, areaName, patternName)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimation"}, err
	}

	var vid int
	vid, err = layoutparams.GetVIDFromDrawAreas(as.AppName, areaName)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimation"}, err
	}

	var srcVlayer layoutcore.VirtualLayer
	srcVlayer, err = layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimation"}, err
	}

	layoutanimation.StartAnimation(srcVlayer, as, areaName, s, nil)
	return &uhmi.Response{Status: "Animation applied successfully"}, nil
}

func runAsyncAnimation(cN clientNotification, work func()) {
	go func() {
		work()
		sendNotification(cN)
	}()
}

func (s *Server) StartSystemAnimationAsync(ctx context.Context, req *uhmi.StartSystemAnimationAsyncRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	areaName := req.GetAreaName()
	patternName := req.GetPatternName()
	requestId := req.GetRequestId()
	clientId, _ := getClientIdFromPeer(ctx)
	err := addRequestWg(clientId, requestId)
	if err != nil {
		return &uhmi.Response{Status: "Animation applied async failed"}, err
	}
	completed := false
	defer func() {
		if !completed {
			asyncReqReg.complete(requestId)
		}
	}()
	cN := clientNotification{
		RequestId: requestId,
		ClientId:  clientId,
		Command:   "StartSystemAnimationAsync",
	}
	as, err := layoutparams.GetAnimationSetting(appName, areaName, patternName)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimationAsync"}, err
	}

	var vid int
	vid, err = layoutparams.GetVIDFromDrawAreas(as.AppName, areaName)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimationAsync"}, err
	}
	var srcVlayer layoutcore.VirtualLayer
	srcVlayer, err = layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return &uhmi.Response{Status: "Failed to StartSystemAnimationAsync"}, err
	}

	runAsyncAnimation(cN, func() {
		layoutanimation.StartAnimation(srcVlayer, as, areaName, s, nil)
	})
	completed = true
	return &uhmi.Response{Status: "Animation applied async successfully"}, nil
}

func (s *Server) WaitAsyncAnimation(ctx context.Context, req *uhmi.WaitAsyncAnimationRequest) (*uhmi.Response, error) {
	serverutil.LogFunc()
	requestId := req.GetRequestId()
	cWg, hasWg := asyncReqReg.get(requestId)
	if hasWg {
		cWg.requestWg.Wait()
		asyncReqReg.remove(requestId)
	} else {
		DLog.Println("The specified async animation has already finished")
	}
	return &uhmi.Response{Status: "async animations waited successfully"}, nil
}

func (s *Server) WaitAllAsyncAnimations(ctx context.Context, req *uhmi.Empty) (*uhmi.Response, error) {
	serverutil.LogFunc()
	for requestId, cWg := range asyncReqReg.snapshot() {
		cWg.requestWg.Wait()
		asyncReqReg.remove(requestId)
	}
	return &uhmi.Response{Status: "All async animations waited successfully"}, nil
}

func (s *Server) GetAnimationInfo(ctx context.Context, req *uhmi.GetAnimationInfoRequest) (*uhmi.GetAnimationInfoResponse, error) {
	serverutil.LogFunc()
	appName := req.GetAppName()
	areaName := req.GetAreaName()
	patternName := req.GetPatternName()
	as, err := layoutparams.GetAnimationSetting(appName, areaName, patternName)
	if err != nil {
		return &uhmi.GetAnimationInfoResponse{Status: "Failed to LayoutGetAnimationInfo"}, err
	}
	var vid int
	vid, err = layoutparams.GetVIDFromDrawAreas(as.AppName, areaName)
	if err != nil {
		return &uhmi.GetAnimationInfoResponse{Status: "Failed to LayoutGetAnimationInfo"}, err
	}
	var vlayer layoutcore.VirtualLayer
	vlayer, err = layoutvscreen.VScreen.GetVlayerParams(vid)
	if err != nil {
		return &uhmi.GetAnimationInfoResponse{Status: "Failed to LayoutGetAnimationInfo"}, err
	}
	angles := layoutparams.GetAnimationAngles(vlayer, as.TimeLine, true)
	timeFrames := layoutparams.GetTimeFrames(as.TimeLine)
	animationInfo, err := layoutparams.GenerateJsonStringFromMap(map[string]interface{}{
		"angles":      angles,
		"time_frames": timeFrames,
	})
	if err != nil {
		return &uhmi.GetAnimationInfoResponse{Status: "Failed to LayoutGetAnimationInfo"}, err
	}
	resp := &uhmi.GetAnimationInfoResponse{
		Status: "Get Animation Info command successfully",
		Info:   animationInfo,
	}

	return resp, nil
}
