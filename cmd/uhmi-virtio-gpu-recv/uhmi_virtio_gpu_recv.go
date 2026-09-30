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
	"unified-hmi/internal/config"
	"unified-hmi/internal/lifecycle/rvgpulauncher"
	. "unified-hmi/internal/ulog"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "%s [option]\n", os.Args[0])

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

func main() {

	flag.Usage = printUsage

	var (
		verbose bool
		debug   bool
	)
	opts := rvgpulauncher.DefaultReceiverOptions()

	fixBoolArgs("vinfo", "vdbg", "a", "v", "l", "L")

	flag.BoolVar(&verbose, "vinfo", true, "verbose info log")
	flag.BoolVar(&debug, "vdbg", false, "verbose debug log (default false)")

	flag.StringVar(&opts.Color, "B", opts.Color, "receiver color of initial screen in RGBA")
	flag.StringVar(&opts.Box, "b", opts.Box, "receiver scanout box")
	flag.StringVar(&opts.SfcId, "i", opts.SfcId, "receiver scanout window ID")

	flag.StringVar(&opts.Card, "g", opts.Card, "receiver use GBM mode on card (default null)")

	flag.StringVar(&opts.Domain, "d", opts.Domain, "receiver domain name for unix socket")

	flag.StringVar(&opts.Seat, "S", opts.Seat, "receiver specify seat for input in GBM mode (default null)")

	flag.StringVar(&opts.Output, "f", opts.Output, "receiver set output id for fullscreen mode on Wayland")
	flag.StringVar(&opts.Port, "p", opts.Port, "receiver port for listening")
	flag.StringVar(&opts.Fps, "V", opts.Fps, "receiver vsync framerate")
	flag.StringVar(&opts.DumpFile, "F", opts.DumpFile, "receiver dump FPS and frame time measurements into file (default null)")
	flag.BoolVar(&opts.Translucent, "a", opts.Translucent, "receiver translucent option")
	flag.BoolVar(&opts.Vsync, "v", opts.Vsync, "receiver vsync option")
	flag.BoolVar(&opts.LayoutMode, "l", opts.LayoutMode, "receiver layout draw mode option")
	flag.BoolVar(&opts.LayoutSync, "L", opts.LayoutSync, "receiver layout sync mode (default false)")


	flag.Parse()

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}
	if debug == true {
		DLog.SetOutput(os.Stderr)
	}
	SetLogPrefix("uhmi-virtio-gpu-recv")

	sigChan := make(chan os.Signal, 1)
	pidChan := make(chan int, 2)
	go config.SignalHandler(sigChan, pidChan, true)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM)

	session, err := rvgpulauncher.StartReceiver(opts)
	if err != nil {
		ELog.Println(err)
		os.Exit(1)
	}

	pidChan <- session.Pid()
	session.Wait()

	ILog.Println("I'm finish")
}
