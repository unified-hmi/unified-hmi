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

import "C"
import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unified-hmi/internal/config"
	"unified-hmi/internal/layout/backend"
	"unified-hmi/internal/layout/core"
	"unified-hmi/internal/layout/rvgpuwinmgr"
	"unified-hmi/internal/lifecycle"
	"unified-hmi/internal/lifecycle/rvgpulauncher"
	. "unified-hmi/internal/ulog"
)

var gFrameworkNodeEnv []string

type sender_t = rvgpulauncher.Sender
type receiver_t = rvgpulauncher.Receiver
type lifecycleCommand = rvgpulauncher.Command

func launcherExists(launcher string, nodes []config.LauncherNode) bool {
	for _, node := range nodes {
		if launcher == node.HostName {
			return true
		}
	}
	return false
}

func validateReceiver(elem map[string]interface{}, nodes []config.LauncherNode) bool {
	if displayArea, ok := elem["display_area"].(string); ok && displayArea != "" {
		return true
	}
	if launcher, ok := elem["launcher"].(string); ok && launcher != "" {
		return launcherExists(launcher, nodes)
	}
	return false
}

func validateAppInfo(mJson map[string]interface{}, nodes []config.LauncherNode) bool {
	fmt2, ok := mJson["format_v1"].(map[string]interface{})
	if !ok {
		return false
	}
	cmd, ok := fmt2["command_type"].(string)
	if !ok {
		return false
	}

	switch cmd {
	case "remote_virtio_gpu", "transport":
		sender, ok := fmt2["sender"].(map[string]interface{})
		if !ok {
			return false
		}
		launcher, ok := sender["launcher"].(string)
		if !ok || !launcherExists(launcher, nodes) {
			return false
		}
		receivers, ok := fmt2["receivers"].([]interface{})
		if !ok {
			return false
		}
		for _, recv := range receivers {
			recvMap, ok := recv.(map[string]interface{})
			if !ok || !validateReceiver(recvMap, nodes) {
				return false
			}
		}
		return true

	case "local":
		l, ok := fmt2["local"].(map[string]interface{})
		return ok && launcherExists(l["launcher"].(string), nodes)

	default:
		return false
	}
}

func getExecutableAppList(recvData []byte) (string, error) {
	var nodes []config.LauncherNode
	if err := json.Unmarshal(recvData, &nodes); err != nil {
		return "", err
	}

	lifecyclepath := config.LifecycleAppDir()
	if !strings.HasSuffix(lifecyclepath, "/") {
		lifecyclepath += "/"
	}
	dirs, err := ioutil.ReadDir(lifecyclepath)
	if err != nil {
		return "", err
	}

	var hitDirs []string
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		dirName := dir.Name()
		fname := filepath.Join(lifecyclepath, dirName, "app.json")

		jsonBytes, err := ioutil.ReadFile(fname)
		if err != nil {
			continue
		}
		var mJson map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &mJson); err != nil {
			continue
		}
		if !validateAppInfo(mJson, nodes) {
			continue
		}

		hitDirs = append(hitDirs, dirName)
	}

	if len(hitDirs) > 0 {
		DLog.Printf("hitDirs %s", strings.Join(hitDirs, ","))
		return strings.Join(hitDirs, ","), nil
	} else {
		return "", errors.New("App.json not found")
	}
}

func ReadAppComm(appName string) ([]byte, error) {
	lifecyclepath := config.LifecycleAppDir()
	if !strings.HasSuffix(lifecyclepath, "/") {
		lifecyclepath += "/"
	}
	fname := lifecyclepath + appName + "/app.json"

	jsonBytes, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

func makeLocalCmdParams(lifecycleComm lifecycleCommand, makeParam string) string {
	makeParam = makeParam + " " + lifecycleComm.FormatV1.Local.Appli
	return makeParam
}

func convertEnvVars(envString string) []string {
	var envSegments []string
	insideSingleQuotes := false
	insideDoubleQuotes := false
	parseType := "key"
	envKey := ""
	envValue := ""

	currentSegment := ""

	if len(envString) == 0 {
		return envSegments
	}

	for _, char := range envString {
		currentChar := string(char)

		if currentChar == "\"" {
			insideDoubleQuotes = !insideDoubleQuotes
		} else if currentChar == "'" {
			insideSingleQuotes = !insideSingleQuotes
		}

		if !insideDoubleQuotes && !insideSingleQuotes {
			if currentChar == "=" && parseType == "key" {
				currentSegment += currentChar
				envKey = currentSegment
				currentSegment = ""
				parseType = "value"
			} else if currentChar == " " && len(currentSegment) > 0 && parseType == "value" {
				envValue = currentSegment
				envSegments = append(envSegments, envKey+envValue)
				currentSegment = ""
				parseType = "key"
				envKey = ""
				envValue = ""
			} else if currentChar == " " {
				continue
			} else {
				currentSegment += currentChar
			}
		} else {
			currentSegment += currentChar
		}
	}

	if parseType == "value" {
		envValue = currentSegment
		envSegments = append(envSegments, envKey+envValue)
	}

	for i, envSegment := range envSegments {
		parts := strings.SplitN(envSegment, "=", 2)
		key := parts[0]
		value := parts[1]

		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		envSegments[i] = key + "=" + value
	}

	return envSegments
}

func addEnvVarIfMissing(execCommEnv []string, varName string) []string {
	for _, env := range execCommEnv {
		if strings.HasPrefix(env, varName+"=") {
			return execCommEnv
		}
	}
	if value := os.Getenv(varName); value != "" {
		execCommEnv = append(execCommEnv, varName+"="+value)
	}

	return execCommEnv
}

func applyFrameworkNodeEnvFallback(execCommEnv []string) []string {
	if len(execCommEnv) == 0 && len(gFrameworkNodeEnv) > 0 {
		return append(execCommEnv, gFrameworkNodeEnv...)
	}
	return execCommEnv
}

func getSenderEnv(sender *sender_t) string {
	if sender.AppliEnv != "" {
		return sender.AppliEnv
	}
	return sender.Env
}

func buildExecCommInfo(lifecycleComm lifecycleCommand) (lifecycle.ExecCommInfo, bool) {
	var execCommInfo lifecycle.ExecCommInfo
	var execComm string
	var execCommEnv []string
	var isWaitDeps bool

	switch lifecycleComm.FormatV1.CommandType {
	case "remote_virtio_gpu":
		if sender := lifecycleComm.FormatV1.Sender; sender != nil {
			execCommEnv = convertEnvVars(getSenderEnv(sender))
			isWaitDeps = true

			if sender.Command != "" {
				args := rvgpulauncher.SenderArgs(lifecycleComm)
				execComm = sender.Command + " " + strings.Join(args, " ")
			} else {
				execCommInfo.LaunchTarget = lifecycle.LaunchTargetRvgpuSend
				execCommInfo.RvgpuSendOptions = rvgpulauncher.SenderOptionsFromCommand(lifecycleComm)
			}
		} else if len(lifecycleComm.FormatV1.Receivers) > 0 {
			Receiver := lifecycleComm.FormatV1.Receivers[0]
			execCommEnv = convertEnvVars(Receiver.Env)
			execCommEnv = applyFrameworkNodeEnvFallback(execCommEnv)
			execCommEnv = addEnvVarIfMissing(execCommEnv, "XDG_RUNTIME_DIR")
			execCommEnv = addEnvVarIfMissing(execCommEnv, "WAYLAND_DISPLAY")
			isWaitDeps = false

			if Receiver.Command != "" {
				execComm = Receiver.Command + " " + strings.Join(rvgpulauncher.ReceiverArgs(lifecycleComm, Receiver), " ")
			} else {
				execCommInfo.LaunchTarget = lifecycle.LaunchTargetRvgpuRecv
				execCommInfo.RvgpuRecvOptions = rvgpulauncher.ReceiverOptionsFromCommand(lifecycleComm, Receiver)
			}
		}
	case "transport":
		if sender := lifecycleComm.FormatV1.Sender; sender != nil && sender.Command != "" {
			execComm = sender.Command
			execCommEnv = convertEnvVars(getSenderEnv(sender))
			isWaitDeps = true
		} else if len(lifecycleComm.FormatV1.Receivers) > 0 && lifecycleComm.FormatV1.Receivers[0].Command != "" {
			Receiver := lifecycleComm.FormatV1.Receivers[0]
			execComm = Receiver.Command
			execCommEnv = convertEnvVars(Receiver.Env)
			execCommEnv = applyFrameworkNodeEnvFallback(execCommEnv)
			isWaitDeps = false
		}

	case "local":
		makeParam := makeLocalCmdParams(lifecycleComm, "")
		execComm = lifecycleComm.FormatV1.Local.Command + makeParam
		execCommEnv = convertEnvVars(lifecycleComm.FormatV1.Local.Env)
		isWaitDeps = true

	default:
		ELog.Printf("command_type: %s is not supported \n", lifecycleComm.FormatV1.CommandType)
		return execCommInfo, false
	}

	execCommInfo.ExecComm = execComm
	execCommInfo.ExecCommEnv = append([]string(nil), execCommEnv...)
	execCommInfo.IsWaitDeps = isWaitDeps
	return execCommInfo, true
}

func handleLifecycleConnection(conn net.Conn, command []byte) {
	defer conn.Close()

	var lifecycleComm lifecycleCommand
	err := json.Unmarshal(command, &lifecycleComm)
	if err != nil {
		return
	}

	appName := lifecycleComm.FormatV1.AppName
	DLog.Printf("appName = %s", appName)

	execCommInfo, ok := buildExecCommInfo(lifecycleComm)
	if !ok {
		return
	}

	commTaskCtx := lifecycle.NewCommTaskCtx(appName)
	defer commTaskCtx.Cancel()

	var subWg sync.WaitGroup
	waitNCountChan := make(chan []byte, 1)
	waitNKeepChan := make(chan int, 1)
	sendLcmChan := make(chan []byte, 2)

	subWg.Add(1)
	go lifecycle.TimingWrapper(execCommInfo, sendLcmChan, waitNCountChan, commTaskCtx, &subWg)

	subWg.Add(1)
	go lifecycle.NkeepWorker(sendLcmChan, waitNKeepChan, commTaskCtx, &subWg)

	var cp lifecycle.ConsistencyProtocol
	config.FramePump(commTaskCtx.Ctx, conn, sendLcmChan, config.PumpHandlers{
		MaxFrame: lifecycle.MaxCommandSize,
		OnMessage: func(recvMsg []byte) bool {
			DLog.Printf("recv %s\n", recvMsg)
			json.Unmarshal(recvMsg, &cp)
			switch cp.CommType {
			case "ncount":
				waitNCountChan <- recvMsg
			case "nkeep":
				waitNKeepChan <- 1
			default:
				return false
			}
			return true
		},
		OnClosed: func() {
			ILog.Printf("(task=%s) Disconnected from the LCM side", commTaskCtx.AppName)
			commTaskCtx.Cancel()
		},
		OnSendError: func(err error) {
			ELog.Printf("(task=%s) ERR ConnWriteWithSize : %s\n", commTaskCtx.AppName, err)
		},
	})

	subWg.Wait()
}

func dispatchLifecycleComm(conn net.Conn) error {
	commType, recvBuf, err := lifecycle.ReadCommand(conn)
	if err != nil {
		conn.Close()
		return err
	}

	switch string(commType) {
	case lifecycle.CMD_GetAppComm:
		DLog.Printf("getAppCmd: %s", string(recvBuf))

		appComm, err := ReadAppComm(string(recvBuf))
		if err != nil {
			ILog.Printf("(task=%s) getAppCmd: %s", recvBuf, err)
		} else {
			DLog.Printf("(task=%s) getAppCmd: %s", recvBuf, appComm)
			lifecycle.ConnWriteWithSize(conn, appComm)
		}

		conn.Close()

	case lifecycle.CMD_ListAvailableApps:
		DLog.Printf("listAvailableApps: %s", string(recvBuf))

		appList, err := getExecutableAppList(recvBuf)
		if err != nil {
			ILog.Printf("listAvailableApps: %s", err)
		} else {
			ILog.Printf("listAvailableApps: %s", appList)
			lifecycle.ConnWriteWithSize(conn, []byte(appList))
		}

		conn.Close()

	case lifecycle.CMD_DistribComm:
		go handleLifecycleConnection(conn, recvBuf)

	default:
		ELog.Printf("commType invalid err: %s", commType)
		conn.Close()
	}

	return nil
}

func lifecycleAcceptLoop(listenAddr string) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		ELog.Printf("LIFECYCLE Listen error: %s", err)
		os.Exit(1)
	}
	defer listener.Close()

	ILog.Printf("LIFECYCLE listening on %s", listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			ELog.Printf("Accept error: %s", err)
			continue
		}
		DLog.Printf("LIFECYCLE socket accepted")
		go dispatchLifecycleComm(conn)
	}
}

var layoutMutex struct {
	sync.Mutex
}

func layoutReadConnection(conn net.Conn) ([]byte, uint32, error) {
	recvBuf, err := config.ReadFrame(conn, 0)
	if err != nil {
		return nil, 0, err
	}
	return recvBuf, uint32(len(recvBuf)), nil
}

func layoutReadCommandLoop(conn net.Conn, jsonChan chan map[string]interface{}, listenerId int, retChansMap map[int]interface{}) {
	defer conn.Close()

	for {
		recvBuf, recvSize, err := layoutReadConnection(conn)
		if err != nil {
			if err == io.EOF {
				DLog.Printf("LAYOUT zero byte read(maybe Client closed the connection)\n")
			} else {
				ELog.Printf("LAYOUT command Read Fail: %s \n", err)
			}
			break
		}
		mJson := make(map[string]interface{})
		err = json.Unmarshal([]byte(string(recvBuf[:recvSize])), &mJson)
		if err != nil {
			ELog.Printf("Unmarshal json command error: %s \n", err)
			break
		}

		mJson["listener_id"] = listenerId

		jsonChan <- mJson

		layoutMutex.Lock()
		retChan := retChansMap[listenerId].(chan map[string]interface{})
		layoutMutex.Unlock()
		select {
		case retJson := <-retChan:
			retBuf, _ := json.Marshal(retJson)
			if err := config.WriteFrame(conn, retBuf); err != nil {
				ELog.Printf("Write error: %s \n", err)
			}
			break
		}
	}
	layoutMutex.Lock()
	delete(retChansMap, listenerId)
	layoutMutex.Unlock()
}

func layoutProcessCommandLoop(
	nodeId int,
	reqChan chan layoutbackend.LocalCommandReq,
	respChan chan layoutbackend.LocalCommandReq,
	jsonChan chan map[string]interface{},
	retChansMap map[int]interface{},
	plugin layoutbackend.LocalCommandGenerator,
) {
	spscrns := new(layoutcore.NodePixelScreens)
	for {
		var mJson map[string]interface{}
		select {
		case mJson = <-jsonChan:
			break
		}

		ret := 0
		listenerId := mJson["listener_id"].(int)
		jsonBytes, err := json.Marshal(mJson)
		if err != nil {
			ret = -1
			layoutCommResponseResult(ret, listenerId, retChansMap)
			continue
		}

		acdata := new(layoutcore.ApplyCommandData)
		err = json.Unmarshal(jsonBytes, acdata)
		if err != nil {
			ret = -1
			layoutCommResponseResult(ret, listenerId, retChansMap)
			continue
		}

		reqs, err := plugin.GenerateLocalCommandReq(acdata, spscrns)
		if err != nil {
			ret = -1
			layoutCommResponseResult(ret, listenerId, retChansMap)
			continue
		}

		ret = layoutSubmitCommand(reqs, reqChan, respChan)

		spscrns = acdata.NPScreens

		layoutCommResponseResult(ret, listenerId, retChansMap)
	}
}

func layoutSubmitCommand(
	reqs []*layoutbackend.LocalCommandReq,
	reqChan chan layoutbackend.LocalCommandReq,
	respChan chan layoutbackend.LocalCommandReq,
) int {
	ret := 0
	for _, req := range reqs {
		reqChan <- *req
		select {
		case lcr := <-respChan:
			ret = lcr.Ret
			break
		}
	}
	return ret
}

func layoutCommResponseResult(
	result int,
	listenerId int,
	retChansMap map[int]interface{},
) {
	layoutMutex.Lock()
	respChan := retChansMap[listenerId].(chan map[string]interface{})
	layoutMutex.Unlock()
	retJson := map[string]interface{}{
		"type":   "result",
		"result": result,
	}
	respChan <- retJson
}

func layoutMainLoop(
	listener net.Listener,
	nodeId int,
	reqChan chan layoutbackend.LocalCommandReq,
	respChan chan layoutbackend.LocalCommandReq,
	plugin layoutbackend.LocalCommandGenerator) {

	jsonChan := make(chan map[string]interface{}, 1)
	retChansMap := make(map[int]interface{})
	go layoutProcessCommandLoop(nodeId, reqChan, respChan, jsonChan, retChansMap, plugin)

	listenerId := 0
	for {
		conn, err := listener.Accept()
		if err != nil {
			ELog.Printf("LAYOUT Accept error: %s", err)
			continue
		}
		retChan := make(chan map[string]interface{}, 1)
		layoutMutex.Lock()
		retChansMap[listenerId] = retChan
		layoutMutex.Unlock()
		go layoutReadCommandLoop(conn, jsonChan, listenerId, retChansMap)
		listenerId += 1
	}
}

func layoutAcceptLoop(vscrnDef *config.VScrnDef, nodeId int, listenAddr string, hostname string) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		ELog.Printf("LAYOUT Listen error: %s", err)
		os.Exit(1)
	}
	defer listener.Close()

	ILog.Printf("LAYOUT listening on %s", listenAddr)

	reqChan := make(chan layoutbackend.LocalCommandReq, 5)
	respChan := make(chan layoutbackend.LocalCommandReq, 5)

	var plugin layoutbackend.LocalCommandGenerator
	rvgpuwinmgr.InitRvgpuWinmgr(vscrnDef, nodeId, hostname)
	plugin = rvgpuwinmgr.RvgpuPlugin{}
	go plugin.Start(reqChan, respChan)

	layoutMainLoop(listener, nodeId, reqChan, respChan, plugin)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, " %s [OPTIONS]\n\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "Options\n")
	flag.PrintDefaults()
}

func startUhmiWorkerNode(vScrnDefFile string, keyHostName string, keyIpAddr string, verbose bool, debug bool) error {
	registerPlatformLauncher()

	if verbose {
		ILog.SetOutput(os.Stderr)
	}
	if debug {
		DLog.SetOutput(os.Stderr)
	}

	vscrnDef, err := config.ReadVScrnDef(vScrnDefFile)
	if err != nil {
		return fmt.Errorf("ReadVScrnDef error: %w", err)
	}

	hostname := keyHostName
	if hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("os.Hostname error: %w", err)
		}
	}

	listenIp := keyIpAddr

	nodeId, err := vscrnDef.GetNodeIdByHostName(hostname)
	if err != nil {
		return fmt.Errorf("GetNodeIdByHostName error: %w", err)
	}

	gFrameworkNodeEnv = vscrnDef.GetFrameworkNodeEnv(nodeId)

	lifecycleListenPort, err := vscrnDef.GetLifecyclePort(nodeId)
	if err != nil {
		return fmt.Errorf("GetLifecyclePort error: %w", err)
	}
	lifecycleListenAddr := listenIp + ":" + strconv.Itoa(lifecycleListenPort)

	dwnListenPort, err := vscrnDef.GetLayoutPort(nodeId)
	if err != nil {
		return fmt.Errorf("GetLayoutPort error: %w", err)
	}
	layoutListenAddr := listenIp + ":" + strconv.Itoa(dwnListenPort)

	prefix := "uhmi-worker-node-" + strconv.Itoa(nodeId)
	SetLogPrefix(prefix)

	ILog.Printf("LIFECYCLE listen: %s", lifecycleListenAddr)
	ILog.Printf("LAYOUT listen: %s", layoutListenAddr)

	go lifecycleAcceptLoop(lifecycleListenAddr)
	layoutAcceptLoop(vscrnDef, nodeId, layoutListenAddr, hostname)

	return nil
}

func main() {
	flag.Usage = printUsage

	var (
		verbose      bool
		debug        bool
		vScrnDefFile string
	)

	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")
	flag.StringVar(&vScrnDefFile, "f", config.VScreenDefPath(), "virtual-screen-def.json file Path")
	flag.Parse()

	if verbose {
		ILog.SetOutput(os.Stderr)
	}
	if debug {
		DLog.SetOutput(os.Stderr)
	}

	if err := startUhmiWorkerNode(vScrnDefFile, "", "", verbose, debug); err != nil {
		ELog.Println(err)
		os.Exit(1)
	}
}

//export StartUhmiWorkerNode
func StartUhmiWorkerNode(vScrnDefFile string, keyHostName string, keyIpAddr string) {
	vsd := string([]byte(vScrnDefFile))
	host := string([]byte(keyHostName))
	ip := string([]byte(keyIpAddr))

	go func(v string, h string, i string) {
		if err := startUhmiWorkerNode(v, h, i, true, false); err != nil {
			ELog.Println(err)
		}
	}(vsd, host, ip)
}
