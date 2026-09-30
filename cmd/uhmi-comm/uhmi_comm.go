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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"unified-hmi/internal/config"
	. "unified-hmi/internal/ulog"
	"unified-hmi/proto/grpc/uhmi"
)

func uhmiServerConnection() (*grpc.ClientConn, error) {
	vscrnDef, err := config.ReadVScrnDef()
	if err != nil {
		return nil, err
	}

	targetAddr, err := vscrnDef.GetUhmiServerNodeAddr()
	if err != nil {
		return nil, err
	}

	targetIP, _, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}

	ipAddrs, err := config.GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		ELog.Println("GetIpv4AddrsOfAllInterfaces error : ", err)
		return nil, err
	}

	nodeId, err := vscrnDef.GetMyNodeId()
	var candidateIPs []string
	if err == nil {
		ipAddr, err := vscrnDef.GetIpAddrByNodeIdAndIpCandidateList(ipAddrs, nodeId)
		if err != nil {
			WLog.Println("GetIpAddrByNodeIdAndIpCandidateList error : ", err)
			candidateIPs = ipAddrs
		} else {
			candidateIPs = []string{ipAddr}
		}
	} else {
		candidateIPs = ipAddrs
	}

	if targetIP != "127.0.0.1" {
		filteredIPs := []string{}
		for _, ip := range candidateIPs {
			ipParsed := net.ParseIP(ip)
			if ipParsed != nil && ipParsed.IsLoopback() {
				continue
			}
			filteredIPs = append(filteredIPs, ip)
		}
		candidateIPs = filteredIPs
	}

	var conn *grpc.ClientConn
	var dialErr error

	for _, ip := range candidateIPs {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			WLog.Printf("Invalid IP: %s, skipping", ip)
			continue
		}
		ILog.Printf("Trying to connect from IP: %s to target: %s", parsedIP, targetAddr)
		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP: parsedIP,
			},
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		conn, dialErr = grpc.DialContext(
			context.Background(),
			targetAddr,
			grpc.WithInsecure(),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", addr)
			}),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                10 * time.Second,
				Timeout:             5 * time.Second,
				PermitWithoutStream: true,
			}),
		)

		if dialErr == nil {
			ILog.Printf("Successfully connected from IP: %s", parsedIP)
			break
		} else {
			WLog.Printf("Connection from IP %s failed: %v", parsedIP, dialErr)
		}
	}
	if dialErr != nil {
		return nil, dialErr
	}

	return conn, nil
}

func printUsage() {
	usage := `
Usage:
  uhmi-comm [OPTIONS]

Options:
  -c    Specify an API command (default: none)

        --- System orchestration ---
        StopAll

        --- Application lifecycle ---
        ListAvailableApps
        ListRunningApps
        GetAppStatus               <appName>
        StopApp                    <appName>

        --- App launch & placement ---
        LaunchApp                  <appName> [destName] [async]
        LaunchApps                 <appName1:destName1> [appName2:destName2 ...]
        MoveWindow                 <appName> <destName>
        MoveWindows                <appName1:destName1> [appName2:destName2 ...]
        WidenApp                   <appName> <dispName1> <dispName2>
        ListDestinations

        --- App mirroring ---
        CreateAppMirror            <appName> <destName>
        RemoveAppMirror            <mirrorName>

        --- Window activation & priority ---
        ActivateWindow             <appName>
        DeactivateWindow           <appName>
        SetAppPriorityGroup        <appName> <group>
        ListPriorityGroups

  -h    Show this message
`
	fmt.Println(usage)
}

func main() {
	var command string

	flag.Usage = printUsage
	flag.StringVar(&command, "c", "", "Specify an API command")
	flag.Parse()

	if command == "" {
		printUsage()
		os.Exit(1)
	}

	ILog.SetOutput(os.Stderr)

	conn, err := uhmiServerConnection()
	if err != nil {
		ELog.Println("uhmiServerConnection: ", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Fprintf(os.Stderr, "ok connected to uhmi-master-node\n")

	args := flag.Args()
	val := handleCommand(conn, command, args)

	fmt.Fprintf(os.Stderr, "exit result: %s\n", resultLabel(val))

	// os.Exit skips deferred calls, so close the connection explicitly.
	// Every non-zero result is normalized to exit code 1 (0 = success).
	conn.Close()
	if val != 0 {
		os.Exit(1)
	}
}

func handleCommand(conn *grpc.ClientConn, command string, args []string) int {
	client := uhmi.NewUHMIServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch command {
	// ===== Orchestration =====
	case "StopAll":
		resp, err := client.StopAll(ctx, &uhmi.Empty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "StopAll error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "StopAll status: %s\n", resp.GetStatus())
		fmt.Fprintf(os.Stderr, "StopAll info: %s\n", resp.GetInfo())
		return reportStatus("StopAll", resp.GetStatus())

	// ===== App lifecycle =====
	case "LaunchApp":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "LaunchApp requires at least appName: LaunchApp <appName> [destName] [async]\n")
			return -1
		}
		// Parse an optional trailing "async" keyword, then an optional destName.
		// The keyword is matched at the end first so a destName is never
		// mistaken for the mode flag.
		rest := args[1:]
		async := false
		if len(rest) > 0 && strings.EqualFold(rest[len(rest)-1], "async") {
			async = true
			rest = rest[:len(rest)-1]
		}
		destName := ""
		if len(rest) >= 1 {
			destName = rest[0]
		}
		// A synchronous launch blocks until the app process exits, so use a
		// cancel-only context with no deadline (like RunApp). An asynchronous
		// launch returns quickly, so a bounded timeout is sufficient.
		var launchCtx context.Context
		var launchCancel context.CancelFunc
		if async {
			launchCtx, launchCancel = context.WithTimeout(context.Background(), 60*time.Second)
		} else {
			cancel()
			launchCtx, launchCancel = context.WithCancel(context.Background())
		}
		defer launchCancel()
		resp, err := client.LaunchApp(launchCtx, &uhmi.LaunchAppRequest{
			AppName: args[0], DestName: destName, Async: async,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "LaunchApp error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "LaunchApp status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("LaunchApp", resp.GetStatus())

	case "StopApp":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "StopApp requires appName\n")
			return -1
		}
		resp, err := client.StopApp(ctx, &uhmi.AppControlRequest{AppName: args[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "StopApp error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "StopApp status: %s\n", resp.GetStatus())
		return reportStatus("StopApp", resp.GetStatus())

	case "GetAppStatus":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "GetAppStatus requires appName\n")
			return -1
		}
		resp, err := client.GetAppStatus(ctx, &uhmi.AppControlRequest{AppName: args[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetAppStatus error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "GetAppStatus status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("GetAppStatus", resp.GetStatus())

	case "ListAvailableApps":
		resp, err := client.ListAvailableApps(ctx, &uhmi.Empty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListAvailableApps error: %v\n", err)
			return -1
		}
		for _, app := range formatAvailableAppList(resp.GetInfo()) {
			fmt.Printf("%-30s  source=%s\n", app.name, app.source)
		}
		return reportStatus("ListAvailableApps", resp.GetStatus())

	case "ListRunningApps":
		resp, err := client.ListRunningApps(ctx, &uhmi.Empty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListRunningApps error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "runningAppList: %s\n", resp.GetInfo())
		return reportStatus("ListRunningApps", resp.GetStatus())

	// ===== Window management =====
	case "MoveWindow":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "MoveWindow requires at least 2 arguments: appName destName [durationMs]\n")
			return -1
		}
		var durationMs float64 = 1000
		if len(args) >= 3 {
			durationMs, _ = strconv.ParseFloat(args[2], 64)
		}
		const defaultCurveId = 3
		resp, err := client.MoveWindow(ctx, &uhmi.MoveWindowRequest{
			AppName: args[0], DestName: args[1],
			DurationMs: durationMs, CurveId: defaultCurveId,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "MoveWindow error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "MoveWindow status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("MoveWindow", resp.GetStatus())

	case "WidenApp":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "WidenApp requires 3 arguments: appName dispName1 dispName2\n")
			return -1
		}
		const widenDefaultDurationMs = 0.0
		const widenDefaultCurveId = 3
		resp, err := client.WidenApp(ctx, &uhmi.WidenAppRequest{
			AppName: args[0], DispName1: args[1], DispName2: args[2],
			DurationMs: widenDefaultDurationMs, CurveId: widenDefaultCurveId,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "WidenApp error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "WidenApp status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("WidenApp", resp.GetStatus())

	case "LaunchApps":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "LaunchApps requires at least 1 argument: appName1:destName1 [appName2:destName2 ...]\n")
			return -1
		}
		entries := make([]*uhmi.LaunchAppEntry, 0, len(args))
		for _, pair := range args {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "LaunchApps: invalid pair %q (expected appName:destName)\n", pair)
				return -1
			}
			entries = append(entries, &uhmi.LaunchAppEntry{AppName: parts[0], DestName: parts[1]})
		}
		cancel()
		longCtx, longCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer longCancel()
		resp, err := client.LaunchApps(longCtx, &uhmi.LaunchAppsRequest{Apps: entries})
		if err != nil {
			fmt.Fprintf(os.Stderr, "LaunchApps error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "LaunchApps status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		code := statusToExitCode(resp.GetStatus())
		for _, r := range resp.GetResults() {
			fmt.Fprintf(os.Stderr, "  app=%-20s status=%s info=%s\n", r.GetAppName(), r.GetStatus(), r.GetInfo())
			if rc := statusToExitCode(r.GetStatus()); rc != 0 {
				code = rc
			}
		}
		if code != 0 {
			fmt.Fprintf(os.Stderr, "LaunchApps: one or more apps failed\n")
		}
		return code

	case "MoveWindows":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "MoveWindows requires at least 1 argument: appName1:destName1 [appName2:destName2 ...]\n")
			return -1
		}
		const multiMoveDurationMs = 1000.0
		const multiMoveDefaultCurveId = 3
		winEntries := make([]*uhmi.MoveWindowEntry, 0, len(args))
		for _, pair := range args {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "MoveWindows: invalid pair %q (expected appName:destName)\n", pair)
				return -1
			}
			winEntries = append(winEntries, &uhmi.MoveWindowEntry{
				AppName: parts[0], DestName: parts[1],
				DurationMs: multiMoveDurationMs, CurveId: multiMoveDefaultCurveId,
			})
		}
		resp, err := client.MoveWindows(ctx, &uhmi.MoveWindowsRequest{Windows: winEntries})
		if err != nil {
			fmt.Fprintf(os.Stderr, "MoveWindows error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "MoveWindows status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		code := statusToExitCode(resp.GetStatus())
		for _, r := range resp.GetResults() {
			fmt.Fprintf(os.Stderr, "  app=%-20s status=%s info=%s\n", r.GetAppName(), r.GetStatus(), r.GetInfo())
			if rc := statusToExitCode(r.GetStatus()); rc != 0 {
				code = rc
			}
		}
		if code != 0 {
			fmt.Fprintf(os.Stderr, "MoveWindows: one or more windows failed\n")
		}
		return code

	case "ListDestinations":
		resp, err := client.ListDestinations(ctx, &uhmi.ListDestinationsRequest{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListDestinations error: %v\n", err)
			return -1
		}
		if rc := statusToExitCode(resp.GetStatus()); rc != 0 {
			fmt.Fprintf(os.Stderr, "ListDestinations failed: %s (status=%s)\n", resp.GetInfo(), resp.GetStatus())
			return rc
		}
		tierLabel := []string{"", "virtual_display", "virtual_display_area", "built-in sub-area"}
		for _, d := range resp.GetDestinations() {
			label := ""
			if int(d.GetTier()) < len(tierLabel) {
				label = tierLabel[d.GetTier()]
			}
			fmt.Printf("%-30s  tier=%d (%s)  x=%.0f y=%.0f w=%.0f h=%.0f\n",
				d.GetName(), d.GetTier(), label,
				d.GetVirtualX(), d.GetVirtualY(), d.GetVirtualW(), d.GetVirtualH())
		}
		return 0

	case "CreateAppMirror":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "CreateAppMirror requires 2 arguments: appName destName\n")
			return -1
		}
		cancel()
		longCtx, longCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer longCancel()
		resp, err := client.CreateAppMirror(longCtx, &uhmi.CreateAppMirrorRequest{
			AppName: args[0], DestName: args[1],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "CreateAppMirror error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "CreateAppMirror status: %s info: %s mirror_name: %s\n", resp.GetStatus(), resp.GetInfo(), resp.GetMirrorName())
		return reportStatus("CreateAppMirror", resp.GetStatus())

	case "RemoveAppMirror":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "RemoveAppMirror requires 1 argument: mirrorName\n")
			return -1
		}
		resp, err := client.RemoveAppMirror(ctx, &uhmi.RemoveAppMirrorRequest{
			MirrorName: args[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "RemoveAppMirror error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "RemoveAppMirror status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("RemoveAppMirror", resp.GetStatus())

	case "ActivateWindow":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "ActivateWindow requires 1 argument: appName\n")
			return -1
		}
		resp, err := client.ActivateWindow(ctx, &uhmi.ActivateWindowRequest{AppName: args[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ActivateWindow error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "ActivateWindow status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("ActivateWindow", resp.GetStatus())

	case "DeactivateWindow":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "DeactivateWindow requires 1 argument: appName\n")
			return -1
		}
		resp, err := client.DeactivateWindow(ctx, &uhmi.DeactivateWindowRequest{AppName: args[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "DeactivateWindow error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "DeactivateWindow status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("DeactivateWindow", resp.GetStatus())

	case "SetAppPriorityGroup":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "SetAppPriorityGroup requires 2 arguments: appName group\n")
			return -1
		}
		resp, err := client.SetAppPriorityGroup(ctx, &uhmi.SetAppPriorityGroupRequest{
			AppName: args[0], Group: args[1],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "SetAppPriorityGroup error: %v\n", err)
			return -1
		}
		fmt.Fprintf(os.Stderr, "SetAppPriorityGroup status: %s info: %s\n", resp.GetStatus(), resp.GetInfo())
		return reportStatus("SetAppPriorityGroup", resp.GetStatus())

	case "ListPriorityGroups":
		resp, err := client.ListPriorityGroups(ctx, &uhmi.Empty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListPriorityGroups error: %v\n", err)
			return -1
		}
		if rc := statusToExitCode(resp.GetStatus()); rc != 0 {
			fmt.Fprintf(os.Stderr, "ListPriorityGroups failed: %s (status=%s)\n", resp.GetInfo(), resp.GetStatus())
			return rc
		}
		for _, g := range resp.GetGroups() {
			mark := ""
			if g.GetIsDefault() {
				mark = " (default)"
			}
			fmt.Printf("[%s]  priority=%d%s\n", g.GetName(), g.GetPriority(), mark)
			if len(g.GetLayers()) == 0 {
				fmt.Printf("  (none)\n")
				continue
			}
			for _, l := range g.GetLayers() {
				vis := "hidden"
				if l.GetVisible() {
					vis = "visible"
				}
				fmt.Printf("  z=%-3d  %-30s  vid=%-8d  %-7s  dst=(%.0f,%.0f %.0fx%.0f)\n",
					l.GetZRank(),
					l.GetAppName()+"/"+l.GetAreaName(),
					l.GetVid(),
					vis,
					l.GetDstX(), l.GetDstY(), l.GetDstW(), l.GetDstH())
			}
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		return -1
	}
}
