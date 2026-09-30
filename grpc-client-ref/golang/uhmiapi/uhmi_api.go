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

package uhmiapi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"uhmi-client/proto/grpc/uhmi"
)

// AppEntry specifies one application to launch with LaunchApps.
type AppEntry struct {
	AppName  string
	DestName string
}

// MoveEntry specifies one window to move with MoveWindows.
type MoveEntry struct {
	AppName    string
	DestName   string
	DurationMs float64
	CurveId    int32
}

// UhmiClientInit initializes a gRPC client connection to the UHMI service.
func UhmiClientInit(serverIp string, serverPort int) (*grpc.ClientConn, uhmi.UHMIServiceClient, error) {
	targetAddr := fmt.Sprintf("%s:%d", serverIp, serverPort)

	conn, err := grpc.NewClient(
		targetAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}

	client := uhmi.NewUHMIServiceClient(conn)
	return conn, client, nil
}

func printResponse(apiName string, status string, info string) {
	if info != "" {
		fmt.Printf("%s response: status=%s info=%s\n", apiName, status, info)
	} else {
		fmt.Printf("%s response: status=%s\n", apiName, status)
	}
}

func callError(apiName string, err error) int {
	fmt.Fprintf(os.Stderr, "Error calling %s: %v\n", apiName, err)
	return -1
}

func statusToExitCode(status string) int {
	switch {
	case strings.EqualFold(status, "success"):
		return 0
	case strings.EqualFold(status, "finish"):
		return 0
	case strings.HasSuffix(strings.ToLower(status), "successfully"):
		return 0
	default:
		return -1
	}
}

// UhmiClientStopAll stops the whole UHMI system.
func UhmiClientStopAll(client uhmi.UHMIServiceClient, ctx context.Context) int {
	resp, err := client.StopAll(ctx, &uhmi.Empty{})
	if err != nil {
		return callError("StopAll", err)
	}
	printResponse("StopAll", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientListAvailableApps lists the applications that can be launched.
func UhmiClientListAvailableApps(client uhmi.UHMIServiceClient, ctx context.Context) int {
	resp, err := client.ListAvailableApps(ctx, &uhmi.Empty{})
	if err != nil {
		return callError("ListAvailableApps", err)
	}
	printResponse("ListAvailableApps", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientListRunningApps lists the currently running applications.
func UhmiClientListRunningApps(client uhmi.UHMIServiceClient, ctx context.Context) int {
	resp, err := client.ListRunningApps(ctx, &uhmi.Empty{})
	if err != nil {
		return callError("ListRunningApps", err)
	}
	printResponse("ListRunningApps", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientGetAppStatus gets the status of the specified application.
func UhmiClientGetAppStatus(client uhmi.UHMIServiceClient, ctx context.Context, appName string) int {
	resp, err := client.GetAppStatus(ctx, &uhmi.AppControlRequest{AppName: appName})
	if err != nil {
		return callError("GetAppStatus", err)
	}
	printResponse("GetAppStatus", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientStopApp stops the specified application.
func UhmiClientStopApp(client uhmi.UHMIServiceClient, ctx context.Context, appName string) int {
	resp, err := client.StopApp(ctx, &uhmi.AppControlRequest{AppName: appName})
	if err != nil {
		return callError("StopApp", err)
	}
	printResponse("StopApp", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientLaunchApp launches an application on the specified destination.
// When destName is empty, the initial layout defined in initial_layout.json is used.
// When async is true, the call returns right after the layout is applied
// without waiting for the application to exit.
func UhmiClientLaunchApp(client uhmi.UHMIServiceClient, ctx context.Context, appName string, destName string, async bool) int {
	req := &uhmi.LaunchAppRequest{
		AppName:  appName,
		DestName: destName,
		Async:    async,
	}
	resp, err := client.LaunchApp(ctx, req)
	if err != nil {
		return callError("LaunchApp", err)
	}
	printResponse("LaunchApp", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientLaunchApps launches multiple applications simultaneously (best-effort).
func UhmiClientLaunchApps(client uhmi.UHMIServiceClient, ctx context.Context, apps []AppEntry) int {
	entries := make([]*uhmi.LaunchAppEntry, 0, len(apps))
	for _, app := range apps {
		entries = append(entries, &uhmi.LaunchAppEntry{
			AppName:  app.AppName,
			DestName: app.DestName,
		})
	}
	resp, err := client.LaunchApps(ctx, &uhmi.LaunchAppsRequest{Apps: entries})
	if err != nil {
		return callError("LaunchApps", err)
	}
	printResponse("LaunchApps", resp.GetStatus(), resp.GetInfo())
	ret := statusToExitCode(resp.GetStatus())
	for _, result := range resp.GetResults() {
		fmt.Printf("  %s: status=%s info=%s\n", result.GetAppName(), result.GetStatus(), result.GetInfo())
		if statusToExitCode(result.GetStatus()) != 0 {
			ret = -1
		}
	}
	return ret
}

// UhmiClientMoveWindow moves the window of the specified application to destName.
func UhmiClientMoveWindow(client uhmi.UHMIServiceClient, ctx context.Context, appName string, destName string, durationMs float64, curveId int32) int {
	req := &uhmi.MoveWindowRequest{
		AppName:    appName,
		DestName:   destName,
		DurationMs: durationMs,
		CurveId:    curveId,
	}
	resp, err := client.MoveWindow(ctx, req)
	if err != nil {
		return callError("MoveWindow", err)
	}
	printResponse("MoveWindow", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientMoveWindows moves multiple application windows simultaneously.
func UhmiClientMoveWindows(client uhmi.UHMIServiceClient, ctx context.Context, windows []MoveEntry) int {
	entries := make([]*uhmi.MoveWindowEntry, 0, len(windows))
	for _, win := range windows {
		entries = append(entries, &uhmi.MoveWindowEntry{
			AppName:    win.AppName,
			DestName:   win.DestName,
			DurationMs: win.DurationMs,
			CurveId:    win.CurveId,
		})
	}
	resp, err := client.MoveWindows(ctx, &uhmi.MoveWindowsRequest{Windows: entries})
	if err != nil {
		return callError("MoveWindows", err)
	}
	printResponse("MoveWindows", resp.GetStatus(), resp.GetInfo())
	ret := statusToExitCode(resp.GetStatus())
	for _, result := range resp.GetResults() {
		fmt.Printf("  %s: status=%s info=%s\n", result.GetAppName(), result.GetStatus(), result.GetInfo())
		if statusToExitCode(result.GetStatus()) != 0 {
			ret = -1
		}
	}
	return ret
}

// UhmiClientWidenApp widens an application window across two adjacent virtual displays.
func UhmiClientWidenApp(client uhmi.UHMIServiceClient, ctx context.Context, appName string, dispName1 string, dispName2 string, durationMs float64, curveId int32) int {
	req := &uhmi.WidenAppRequest{
		AppName:    appName,
		DispName1:  dispName1,
		DispName2:  dispName2,
		DurationMs: durationMs,
		CurveId:    curveId,
	}
	resp, err := client.WidenApp(ctx, req)
	if err != nil {
		return callError("WidenApp", err)
	}
	printResponse("WidenApp", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientListDestinations enumerates all valid dest_name values.
func UhmiClientListDestinations(client uhmi.UHMIServiceClient, ctx context.Context) int {
	resp, err := client.ListDestinations(ctx, &uhmi.ListDestinationsRequest{})
	if err != nil {
		return callError("ListDestinations", err)
	}
	printResponse("ListDestinations", resp.GetStatus(), resp.GetInfo())
	for _, dest := range resp.GetDestinations() {
		fmt.Printf("  [tier %d] %s: (%v, %v) %vx%v\n",
			dest.GetTier(), dest.GetName(),
			dest.GetVirtualX(), dest.GetVirtualY(),
			dest.GetVirtualW(), dest.GetVirtualH())
	}
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientCreateAppMirror mirrors a running application to another destination.
func UhmiClientCreateAppMirror(client uhmi.UHMIServiceClient, ctx context.Context, appName string, destName string) int {
	req := &uhmi.CreateAppMirrorRequest{
		AppName:  appName,
		DestName: destName,
	}
	resp, err := client.CreateAppMirror(ctx, req)
	if err != nil {
		return callError("CreateAppMirror", err)
	}
	printResponse("CreateAppMirror", resp.GetStatus(), resp.GetInfo())
	if resp.GetMirrorName() != "" {
		fmt.Println("  mirror_name:", resp.GetMirrorName())
	}
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientRemoveAppMirror removes a previously created mirror layer.
func UhmiClientRemoveAppMirror(client uhmi.UHMIServiceClient, ctx context.Context, mirrorName string) int {
	resp, err := client.RemoveAppMirror(ctx, &uhmi.RemoveAppMirrorRequest{MirrorName: mirrorName})
	if err != nil {
		return callError("RemoveAppMirror", err)
	}
	printResponse("RemoveAppMirror", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientActivateWindow brings the application layers to the front of their priority group.
func UhmiClientActivateWindow(client uhmi.UHMIServiceClient, ctx context.Context, appName string) int {
	resp, err := client.ActivateWindow(ctx, &uhmi.ActivateWindowRequest{AppName: appName})
	if err != nil {
		return callError("ActivateWindow", err)
	}
	printResponse("ActivateWindow", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientDeactivateWindow sends the application layers to the back of their priority group and hides them.
func UhmiClientDeactivateWindow(client uhmi.UHMIServiceClient, ctx context.Context, appName string) int {
	resp, err := client.DeactivateWindow(ctx, &uhmi.DeactivateWindowRequest{AppName: appName})
	if err != nil {
		return callError("DeactivateWindow", err)
	}
	printResponse("DeactivateWindow", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientSetAppPriorityGroup reassigns all layers of an application to the specified priority group.
func UhmiClientSetAppPriorityGroup(client uhmi.UHMIServiceClient, ctx context.Context, appName string, group string) int {
	req := &uhmi.SetAppPriorityGroupRequest{
		AppName: appName,
		Group:   group,
	}
	resp, err := client.SetAppPriorityGroup(ctx, req)
	if err != nil {
		return callError("SetAppPriorityGroup", err)
	}
	printResponse("SetAppPriorityGroup", resp.GetStatus(), resp.GetInfo())
	return statusToExitCode(resp.GetStatus())
}

// UhmiClientListPriorityGroups lists all priority groups with their runtime app assignments.
func UhmiClientListPriorityGroups(client uhmi.UHMIServiceClient, ctx context.Context) int {
	resp, err := client.ListPriorityGroups(ctx, &uhmi.Empty{})
	if err != nil {
		return callError("ListPriorityGroups", err)
	}
	printResponse("ListPriorityGroups", resp.GetStatus(), resp.GetInfo())
	for _, group := range resp.GetGroups() {
		fmt.Printf("  %s (priority=%d, default=%v):\n", group.GetName(), group.GetPriority(), group.GetIsDefault())
		for _, layer := range group.GetLayers() {
			fmt.Printf("    %s/%s: vid=%d z_rank=%d visible=%v dst=(%v, %v, %v, %v)\n",
				layer.GetAppName(), layer.GetAreaName(), layer.GetVid(), layer.GetZRank(),
				layer.GetVisible(), layer.GetDstX(), layer.GetDstY(), layer.GetDstW(), layer.GetDstH())
		}
	}
	return statusToExitCode(resp.GetStatus())
}
