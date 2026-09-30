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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unified-hmi/internal/lifecycle"
	"unified-hmi/internal/lifecycle/rvgpulauncher"
	. "unified-hmi/internal/ulog"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "%s [option] execComm\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "[option]\n")
	flag.PrintDefaults()
}

func fixBoolArgs(names ...string) {
	boolSet := make(map[string]bool)
	for _, name := range names {
		boolSet["-"+name] = true
	}

	isBoolValue := func(s string) bool {
		switch strings.ToLower(s) {
		case "true", "false", "0", "1", "t", "f":
			return true
		}
		return false
	}

	newArgs := []string{os.Args[0]}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if boolSet[arg] && i+1 < len(os.Args) && isBoolValue(os.Args[i+1]) {
			newArgs = append(newArgs, arg+"="+os.Args[i+1])
			i++
		} else {
			newArgs = append(newArgs, arg)
		}
	}
	os.Args = newArgs
}

type terminableSession interface {
	Exited() <-chan struct{}
	Terminate(time.Duration)
}

func terminateSession(session terminableSession, startErr error, termTimeout time.Duration) {
	if startErr != nil {
		session.Terminate(termTimeout)
		return
	}

	go func() {
		<-session.Exited()
		session.Terminate(termTimeout)
	}()
}

func main() {

	flag.Usage = printUsage

	var (
		verbose bool
		debug   bool

		targets     lifecycle.MultiFlag
		termTimeout int
	)
	opts := rvgpulauncher.DefaultSenderOptions()

	fixBoolArgs("vinfo", "vdbg")

	flag.BoolVar(&verbose, "vinfo", true, "verbose info log")
	flag.BoolVar(&debug, "vdbg", false, "verbose debug log (default false)")

	flag.StringVar(&opts.Capset, "c", opts.Capset, "proxy param: capset file path (default null)")
	flag.StringVar(&opts.Scanout, "s", opts.Scanout, "proxy param: scanout in form WxH@X,Y (default 1920x1080@0,0)")
	flag.StringVar(&opts.Framerate, "f", opts.Framerate, "proxy param: virtual framerate (default null)")
	flag.StringVar(&opts.AppName, "i", opts.AppName, "specify rvgpu surface id (default null)")
	flag.Var(&targets, "n", "proxy param: serverIp:port for connectiong (max 4 hosts, defalut 127.0.0.1:55667)")

	flag.IntVar(&termTimeout, "term_timeout", 60, "SIGTERM timeout in seconds before SIGKILL for child processes")

	flag.Parse()

	appArgs := flag.Args()
	if len(appArgs) == 0 {
		ELog.Println("no target app")
		os.Exit(1)
	}

	opts.Targets = targets
	opts.TargetApp = appArgs[0]
	opts.TargetAppArgs = appArgs[1:]

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}

	if debug == true {
		DLog.SetOutput(os.Stderr)
	}

	SetLogPrefix("uhmi-virtio-gpu-wl-send " + opts.TargetApp)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM)

	session, err := rvgpulauncher.StartSender(opts)

	go func() {
		<-sigChan
		session.Terminate(time.Duration(termTimeout) * time.Second)
	}()

	if err != nil {
		ELog.Println(err)
	}
	terminateSession(session, err, time.Duration(termTimeout)*time.Second)

	session.Wait()
	ILog.Println("I'm finish")
}
