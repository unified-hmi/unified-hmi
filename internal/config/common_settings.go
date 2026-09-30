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

package config

import (
	_ "context"
	"errors"
	"net"
	"os"
	"strconv"
)

const (
	STAT_ExecSuccess = "Success"
	STAT_ExecErr     = "Error"
	STAT_ExecFin     = "Finish"
	STAT_ExecBusy    = "Busy"
	STAT_AppRunning  = "AppRunning"
	STAT_AppStop     = "AppStop"
)

func GetEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetIpv4AddrsOfAllInterfaces() ([]string, error) {

	var ipaddrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return ipaddrs, err
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ipv4 := ip.To4()
			if ipv4 != nil {
				ipaddrs = append(ipaddrs, ipv4.String())
			}
		}
	}

	if len(ipaddrs) == 0 {
		return ipaddrs, errors.New("Cannnot Find My IpAddr from ifaces")
	} else {
		return ipaddrs, nil
	}

}

func GetEnvString(key string, fallback string) string {
	return GetEnv(key, fallback)
}

func GetEnvBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	if value == "" {
		return true
	}

	valueInt, err := strconv.Atoi(value)
	if err == nil {
		if valueInt > 0 {
			return true
		} else {
			return false
		}
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return boolValue
}
