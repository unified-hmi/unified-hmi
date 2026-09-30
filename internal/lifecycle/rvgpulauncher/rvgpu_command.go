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

package rvgpulauncher

import (
	"net"
	"strconv"
	"strings"

	"unified-hmi/internal/config"
)

type FrontendParams struct {
	ScanOutX         int     `json:"scanout_x"`
	ScanOutY         int     `json:"scanout_y"`
	ScanOutW         int     `json:"scanout_w"`
	ScanOutH         int     `json:"scanout_h"`
	ServerPort       int     `json:"server_port"`
	SessionTimeOut   int     `json:"session_timeout"`
	DeveloperOptions *string `json:"developer_options"`
}

type BackendParams struct {
	IviSurfaceId       *int    `json:"ivi_surface_id"`
	SockDomainName     *string `json:"sock_domain_name"`
	RDisplayId         *int    `json:"rdisplay_id"`
	ScanOutX           int     `json:"scanout_x"`
	ScanOutY           int     `json:"scanout_y"`
	ScanOutW           int     `json:"scanout_w"`
	ScanOutH           int     `json:"scanout_h"`
	ListenPort         int     `json:"listen_port"`
	InitialScreenColor *string `json:"initial_screen_color"`
	DeveloperOptions   *string `json:"developer_options"`
}

type Sender struct {
	Launcher       config.LauncherNode `json:"launcher"`
	Command        string              `json:"command"`
	FrontendParams FrontendParams      `json:"frontend_params"`
	AppliEnv       string              `json:"appli_env"`
	Env            string              `json:"env"`
	Appli          string              `json:"appli"`
}

type Receiver struct {
	Launcher      config.LauncherNode `json:"launcher"`
	Command       string              `json:"command"`
	BackendParams BackendParams       `json:"backend_params"`
	Env           string              `json:"env"`
}

type Local struct {
	Launcher config.LauncherNode `json:"launcher"`
	Command  string              `json:"command"`
	Env      string              `json:"env"`
	Appli    string              `json:"appli"`
}

type Command struct {
	FormatV1 struct {
		AppName     string     `json:"appli_name"`
		CommandType string     `json:"command_type"`
		Sender      *Sender    `json:"sender"`
		Receivers   []Receiver `json:"receivers"`
		Local       Local      `json:"local"`
	} `json:"format_v1"`
}

func SenderArgs(command Command) []string {
	sender := command.FormatV1.Sender
	params := sender.FrontendParams
	args := []string{"-s", ScanoutSpec(params.ScanOutW, params.ScanOutH, params.ScanOutX, params.ScanOutY)}
	for _, receiver := range command.FormatV1.Receivers {
		args = append(args, "-n", receiver.Launcher.Ip+":"+strconv.Itoa(receiver.BackendParams.ListenPort))
	}
	args = append(args, "-i", command.FormatV1.AppName)
	if params.DeveloperOptions != nil {
		args = append(args, *params.DeveloperOptions)
	}
	if sender.Appli != "" {
		args = append(args, sender.Appli)
	}
	return args
}

func ReceiverArgs(command Command, receiver Receiver) []string {
	params := receiver.BackendParams
	args := []string{}
	if params.IviSurfaceId != nil {
		args = append(args, "-i", strconv.Itoa(*params.IviSurfaceId))
	}
	if params.SockDomainName != nil {
		args = append(args, "-d", *params.SockDomainName)
	}
	if params.RDisplayId != nil {
		args = append(args, "-f", strconv.Itoa(*params.RDisplayId))
	}
	args = append(args, "-b", ScanoutSpec(params.ScanOutW, params.ScanOutH, params.ScanOutX, params.ScanOutY), "-p", strconv.Itoa(params.ListenPort))
	if params.InitialScreenColor != nil {
		args = append(args, "-B", *params.InitialScreenColor)
	}
	if params.DeveloperOptions != nil {
		args = append(args, *params.DeveloperOptions)
	}
	return args
}

func ScanoutSpec(width, height, x, y int) string {
	return strconv.Itoa(width) + "x" + strconv.Itoa(height) + "@" + strconv.Itoa(x) + "," + strconv.Itoa(y)
}

func SenderOptionsFromCommand(command Command) *SenderOptions {
	sender := command.FormatV1.Sender
	opts := DefaultSenderOptions()
	opts.Scanout = ScanoutSpec(sender.FrontendParams.ScanOutW, sender.FrontendParams.ScanOutH, sender.FrontendParams.ScanOutX, sender.FrontendParams.ScanOutY)
	opts.AppName = command.FormatV1.AppName
	opts.DeveloperOptions = sender.FrontendParams.DeveloperOptions
	for _, receiver := range command.FormatV1.Receivers {
		opts.Targets = append(opts.Targets, net.JoinHostPort(receiver.Launcher.Ip, strconv.Itoa(receiver.BackendParams.ListenPort)))
	}
	if application := strings.Fields(sender.Appli); len(application) > 0 {
		opts.TargetApp = application[0]
		opts.TargetAppArgs = application[1:]
	}
	return &opts
}

func ReceiverOptionsFromCommand(command Command, receiver Receiver) *ReceiverOptions {
	params := receiver.BackendParams
	opts := DefaultReceiverOptions()
	opts.Box = ScanoutSpec(params.ScanOutW, params.ScanOutH, params.ScanOutX, params.ScanOutY)
	opts.Port = strconv.Itoa(params.ListenPort)
	if params.IviSurfaceId != nil {
		opts.SfcId = strconv.Itoa(*params.IviSurfaceId)
	}
	if params.SockDomainName != nil {
		opts.Domain = *params.SockDomainName
	}
	if params.RDisplayId != nil {
		opts.Output = strconv.Itoa(*params.RDisplayId)
	}
	if params.InitialScreenColor != nil {
		opts.Color = *params.InitialScreenColor
	}
	opts.DeveloperOptions = params.DeveloperOptions
	return &opts
}

func ApplySenderEnvironment(opts SenderOptions, env []string) SenderOptions {
	opts.TargetAppEnv = env
	if dir, ok := EnvValue(env, "XDG_RUNTIME_DIR"); ok {
		opts.XdgRuntimeDir = dir
	}
	return opts
}
