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
	"os"
	"strings"
	"time"

	"uhmi-client/uhmiapi"
)

func printUsage() {
	usage := `
Usage: uhmi_grpc_client_ref [options] [arguments...]

Options:
  -c string   UHMI API command to execute (default: ListDestinations)
  -s string   server IP address (default: 127.0.0.1)
  -p int      server port (default: 6643)
  -async      LaunchApp: return without waiting for the app to exit
  -d float    animation duration in milliseconds for MoveWindow(s)/WidenApp (default: 0)
  -curve int  animation curve ID for MoveWindow(s)/WidenApp (default: 0)
  -h          show this message

Commands:
  StopAll                          no arguments
  ListAvailableApps                no arguments
  ListRunningApps                  no arguments
  GetAppStatus                     app_name
  StopApp                          app_name
  LaunchApp                        app_name [dest_name] [-async]
  LaunchApps                       app_name:dest_name ...
  MoveWindow                       app_name dest_name [-d ms] [-curve id]
  MoveWindows                      app_name:dest_name ... [-d ms] [-curve id]
  WidenApp                         app_name disp_name1 disp_name2 [-d ms] [-curve id]
  ListDestinations                 no arguments
  CreateAppMirror                  app_name dest_name
  RemoveAppMirror                  mirror_name
  ActivateWindow                   app_name
  DeactivateWindow                 app_name
  SetAppPriorityGroup              app_name group
  ListPriorityGroups               no arguments
`
	fmt.Println(usage)
}

func needArgs(command string, args []string, n int, usage string) bool {
	if len(args) < n {
		fmt.Fprintf(os.Stderr, "Error: %s requires %s\n", command, usage)
		return false
	}
	return true
}

func parseAppDest(arg string) (string, string) {
	appName, destName, _ := strings.Cut(arg, ":")
	return appName, destName
}

var (
	boolFlagNames  = map[string]bool{"async": true, "h": true}
	valueFlagNames = map[string]bool{"c": true, "s": true, "p": true, "d": true, "curve": true}
)

func splitFlagArgs(rawArgs []string) (flagArgs []string, positional []string) {
	for i := 0; i < len(rawArgs); i++ {
		a := rawArgs[i]
		name, isFlag := strings.CutPrefix(a, "-")
		if isFlag {
			name = strings.TrimPrefix(name, "-")
		}
		name, _, hasEq := strings.Cut(name, "=")
		switch {
		case isFlag && boolFlagNames[name]:
			flagArgs = append(flagArgs, a)
		case isFlag && valueFlagNames[name]:
			flagArgs = append(flagArgs, a)
			if !hasEq && i+1 < len(rawArgs) {
				i++
				flagArgs = append(flagArgs, rawArgs[i])
			}
		default:
			positional = append(positional, a)
		}
	}
	return flagArgs, positional
}

func main() {
	var (
		command    string
		serverIp   string
		serverPort int
		async      bool
		durationMs float64
		curveId    int
		showHelp   bool
		ret        int
	)

	flag.StringVar(&command, "c", "ListDestinations", "UHMI API command to execute")
	flag.StringVar(&serverIp, "s", "127.0.0.1", "server IP address")
	flag.IntVar(&serverPort, "p", 6643, "server port")
	flag.BoolVar(&async, "async", false, "LaunchApp: return without waiting for the app to exit")
	flag.Float64Var(&durationMs, "d", 0, "animation duration in milliseconds")
	flag.IntVar(&curveId, "curve", 0, "animation curve ID")
	flag.BoolVar(&showHelp, "h", false, "show this message")
	flag.Usage = printUsage
	flagArgs, args := splitFlagArgs(os.Args[1:])
	flag.CommandLine.Parse(flagArgs)

	if showHelp {
		printUsage()
		os.Exit(0)
	}

	conn, client, err := uhmiapi.UhmiClientInit(serverIp, serverPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calling UhmiClientInit: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch command {
	case "StopAll":
		ret = uhmiapi.UhmiClientStopAll(client, ctx)
	case "ListAvailableApps":
		ret = uhmiapi.UhmiClientListAvailableApps(client, ctx)
	case "ListRunningApps":
		ret = uhmiapi.UhmiClientListRunningApps(client, ctx)
	case "GetAppStatus":
		if !needArgs(command, args, 1, "app_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientGetAppStatus(client, ctx, args[0])
	case "StopApp":
		if !needArgs(command, args, 1, "app_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientStopApp(client, ctx, args[0])
	case "LaunchApp":
		if !needArgs(command, args, 1, "app_name [dest_name]") {
			ret = -1
			break
		}
		destName := ""
		if len(args) >= 2 {
			destName = args[1]
		}
		ret = uhmiapi.UhmiClientLaunchApp(client, ctx, args[0], destName, async)
	case "LaunchApps":
		if !needArgs(command, args, 1, "app_name:dest_name ...") {
			ret = -1
			break
		}
		apps := make([]uhmiapi.AppEntry, 0, len(args))
		for _, arg := range args {
			appName, destName := parseAppDest(arg)
			apps = append(apps, uhmiapi.AppEntry{AppName: appName, DestName: destName})
		}
		ret = uhmiapi.UhmiClientLaunchApps(client, ctx, apps)
	case "MoveWindow":
		if !needArgs(command, args, 2, "app_name dest_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientMoveWindow(client, ctx, args[0], args[1], durationMs, int32(curveId))
	case "MoveWindows":
		if !needArgs(command, args, 1, "app_name:dest_name ...") {
			ret = -1
			break
		}
		windows := make([]uhmiapi.MoveEntry, 0, len(args))
		for _, arg := range args {
			appName, destName := parseAppDest(arg)
			windows = append(windows, uhmiapi.MoveEntry{
				AppName:    appName,
				DestName:   destName,
				DurationMs: durationMs,
				CurveId:    int32(curveId),
			})
		}
		ret = uhmiapi.UhmiClientMoveWindows(client, ctx, windows)
	case "WidenApp":
		if !needArgs(command, args, 3, "app_name disp_name1 disp_name2") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientWidenApp(client, ctx, args[0], args[1], args[2], durationMs, int32(curveId))
	case "ListDestinations":
		ret = uhmiapi.UhmiClientListDestinations(client, ctx)
	case "CreateAppMirror":
		if !needArgs(command, args, 2, "app_name dest_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientCreateAppMirror(client, ctx, args[0], args[1])
	case "RemoveAppMirror":
		if !needArgs(command, args, 1, "mirror_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientRemoveAppMirror(client, ctx, args[0])
	case "ActivateWindow":
		if !needArgs(command, args, 1, "app_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientActivateWindow(client, ctx, args[0])
	case "DeactivateWindow":
		if !needArgs(command, args, 1, "app_name") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientDeactivateWindow(client, ctx, args[0])
	case "SetAppPriorityGroup":
		if !needArgs(command, args, 2, "app_name group") {
			ret = -1
			break
		}
		ret = uhmiapi.UhmiClientSetAppPriorityGroup(client, ctx, args[0], args[1])
	case "ListPriorityGroups":
		ret = uhmiapi.UhmiClientListPriorityGroups(client, ctx)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", command)
		printUsage()
		ret = -1
	}

	if ret != 0 {
		os.Exit(1)
	}
}
