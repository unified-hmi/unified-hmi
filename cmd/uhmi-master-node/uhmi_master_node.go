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
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"unified-hmi/proto/grpc/uhmi"

	"unified-hmi/internal/config"
	"unified-hmi/internal/server/uhmi"

	. "unified-hmi/internal/ulog"
)

var gVScrnDef *config.VScrnDef = nil

func printUsage() {
	usage := `
Usage:
  uhmi-master-node [OPTIONS]

Options:
  -a            app-list-def.json file path
  -f            virtual-screen-def.json file path
  -v            verbose info log (default: true)
  -d            verbose debug log (default: false)
  -h            Show this message
`
	fmt.Println(usage)
}

func applyAppListDefPath(path string) error {
	return os.Setenv(config.EnvAppListDefPath, path)
}

func main() {
	var (
		appListDefFile string
		vScrnDefFile   string
		verbose        bool
		debug          bool
	)

	flag.Usage = printUsage
	flag.StringVar(&appListDefFile, "a", config.AppListDefPath(), "app-list-def.json file path")
	flag.StringVar(&vScrnDefFile, "f", config.VScreenDefPath(), "virtual-screen-def.json file Path")
	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")
	flag.Parse()

	if err := applyAppListDefPath(appListDefFile); err != nil {
		ELog.Printf("Failed to set %s: %v", config.EnvAppListDefPath, err)
		os.Exit(1)
	}

	if verbose {
		ILog.SetOutput(os.Stderr)
	}
	if debug {
		DLog.SetOutput(os.Stderr)
	}

	fmt.Fprintln(os.Stderr, "[uhmi-master-node] started")

	vscrnDef, err := config.ReadVScrnDef(vScrnDefFile)
	if err != nil {
		ELog.Println("ReadVScrnDef error : ", err)
		os.Exit(1)
	}

	listenAddr, err := vscrnDef.GetUhmiServerNodeAddr()
	if err != nil {
		ELog.Println("GetUhmiServerNodeAddr error : ", err)
		os.Exit(1)
	}
	ILog.Printf("listenAddr=%s", listenAddr)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[uhmi-master-node] Failed to listen on %s: %v\n", listenAddr, err)
		os.Exit(1)
	}

	kaep := keepalive.EnforcementPolicy{
		MinTime:             5 * time.Second,
		PermitWithoutStream: true,
	}
	kasp := keepalive.ServerParameters{
		MaxConnectionIdle:     0,
		MaxConnectionAge:      0,
		MaxConnectionAgeGrace: 5 * time.Minute,
		Time:                  5 * time.Second,
		Timeout:               1 * time.Second,
	}
	grpcServer := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(kaep),
		grpc.KeepaliveParams(kasp),
	)

	uhmiSrv, err := uhmiserver.NewUhmiServer(vscrnDef)
	if err != nil {
		ELog.Printf("Failed to init UHMI server: %v", err)
		os.Exit(1)
	}
	uhmi.RegisterUHMIServiceServer(grpcServer, uhmiSrv)

	serveErrCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "[uhmi-master-node] gRPC server listening on %s\n", listenAddr)
		serveErrCh <- grpcServer.Serve(lis)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "[uhmi-master-node] received signal: %v, shutting down\n", sig)
		grpcServer.GracefulStop()
		<-serveErrCh
	case err := <-serveErrCh:
		if err != nil {
			fmt.Fprintf(os.Stderr, "[uhmi-master-node] gRPC server error: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Fprintln(os.Stderr, "[uhmi-master-node] exit")
}
