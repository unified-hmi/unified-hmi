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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"unified-hmi/internal/layout/core"
)

const DEFAULT_LIFECYCLE_PORT = 7654
const DEFAULT_LAYOUT_PORT = 10100

type RealDisplay struct {
	NodeId     int `json:"node_id"`
	VDisplayId int `json:"vdisplay_id"`
	PixelW     int `json:"pixel_w"`
	PixelH     int `json:"pixel_h"`
	RDisplayId int `json:"rdisplay_id"`
}

type VScrnDef struct {
	Def2D struct {
		Size struct {
			VirtualW float64 `json:"virtual_w"`
			VirtualH float64 `json:"virtual_h"`
		} `json:"size"`

		VirtualDisplays []struct {
			DispName   string  `json:"disp_name"`
			VDisplayId int     `json:"vdisplay_id"`
			VirtualX   float64 `json:"virtual_x"`
			VirtualY   float64 `json:"virtual_y"`
			VirtualW   float64 `json:"virtual_w"`
			VirtualH   float64 `json:"virtual_h"`
		} `json:"virtual_displays"`

		VirtualDisplayAreas []struct {
			AreaName string  `json:"area_name"`
			VirtualX float64 `json:"virtual_x"`
			VirtualY float64 `json:"virtual_y"`
			VirtualW float64 `json:"virtual_w"`
			VirtualH float64 `json:"virtual_h"`
		} `json:"virtual_display_areas"`
	} `json:"virtual_screen_2d"`

	RealDisplays []RealDisplay `json:"real_displays"`

	Nodes []struct {
		NodeId   int    `json:"node_id"`
		HostName string `json:"hostname"`
		Ip       string `json:"ip"`
	} `json:"node"`

	DistributedWindowSystem struct {
		UhmiMasterNode struct {
			NodeId int `json:"node_id"`
			Port   int `json:"port"`
		} `json:"uhmi_master_node"`
		UhmiServer struct {
			NodeId int `json:"node_id"`
			Port   int `json:"port"`
		} `json:"uhmi_server"`
		FrameworkNode []struct {
			NodeId        int       `json:"node_id"`
			LifecyclePort int       `json:"lifecycle_port"`
			LayoutPort    int       `json:"layout_port"`
			Env           *[]string `json:"env"`
			Ucl           struct {
				Port int       `json:"port"`
				Env  *[]string `json:"env"`
			} `json:"ucl_node"`
			Ula struct {
				Port int `json:"port"`
			} `json:"ula"`
			Compositor []struct {
				VDisplayIds    []int   `json:"vdisplay_ids"`
				IviSurfaceId   *int    `json:"ivi_surface_id"`
				SockDomainName *string `json:"sock_domain_name"`
				ListenPort     int     `json:"listen_port"`
			} `json:"compositor"`
		} `json:"framework_node"`
	} `json:"distributed_window_system"`

	VirtualSafetyArea []struct {
		VirtualX float64 `json:"virtual_x"`
		VirtualY float64 `json:"virtual_y"`
		VirtualW float64 `json:"virtual_w"`
		VirtualH float64 `json:"virtual_h"`
	} `json:"virtual_safety_area"`
}

type LauncherNode struct {
	Ip       string `json:"ip"`
	Port     int    `json:"port"`
	HostName string `json:"hostname"`
}

type DistribNode struct {
	NodeId     int
	Ip         string
	LayoutPort int
}

func ReadVScrnDef(vsdPath ...string) (*VScrnDef, error) {
	var fname string
	if len(vsdPath) > 0 && vsdPath[0] != "" {
		fname = vsdPath[0]
	} else {
		fname = VScreenDefPath()
	}

	f, err := os.Open(fname)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("%s %s", fname, err))
	}
	defer f.Close()

	jsonBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("ReadAll error: %s", err))
	}

	var vscrnDef VScrnDef
	err = json.Unmarshal(jsonBytes, &vscrnDef)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("json Unmarshal error: %s", err))
	}

	return &vscrnDef, nil
}

func (vdef *VScrnDef) GetNodeIdByHostName(hostname string) (int, error) {

	for _, r := range vdef.Nodes {
		if hostname == r.HostName {
			return r.NodeId, nil
		}
	}

	return -1, errors.New("Cannot Find My NodeId from VScrnDef json")
}

func (vdef *VScrnDef) GetIpAddrByNodeIdAndIpCandidateList(ipAddrs []string, nodeId int) (string, error) {

	for _, r := range vdef.Nodes {
		if nodeId == r.NodeId {
			for _, ipaddr := range ipAddrs {
				if ipaddr == r.Ip {
					return ipaddr, nil
				}
			}
		}
	}

	return "0.0.0.0", errors.New("The acquired IP address does not exist in VScrnDef json")
}

func (vdef *VScrnDef) GetMyNodeId() (int, error) {
	keyHostName, err := os.Hostname()
	if err != nil {
		return -1, err
	}
	nodeId, err := vdef.GetNodeIdByHostName(keyHostName)
	if err != nil {
		return -1, err
	}

	return nodeId, nil
}

func (vdef *VScrnDef) GetMyIpv4Addr() (string, error) {

	keyHostName, err := os.Hostname()
	if err != nil {
		return "", err
	}

	nodeId, err := vdef.GetNodeIdByHostName(keyHostName)
	if err != nil {
		return "", err
	}
	ipAddrs, err := GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		return "", err
	}
	ipAddr, err := vdef.GetIpAddrByNodeIdAndIpCandidateList(ipAddrs, nodeId)
	if err != nil {
		return "", err
	}

	return ipAddr, nil
}

func (vdef *VScrnDef) GetIpAddr(nodeId int) (string, error) {

	for _, r := range vdef.Nodes {
		if nodeId == r.NodeId {
			return r.Ip, nil
		}
	}

	return "0.0.0.0", errors.New("Connot Find My IP Address from VScrnDef json")
}

func (vdef *VScrnDef) GetNodeIdAndIpAddr(ipAddrs []string, hostname string) (int, string, error) {

	for _, r := range vdef.Nodes {
		if hostname == r.HostName {
			for _, ipaddr := range ipAddrs {
				if ipaddr == r.Ip {
					return r.NodeId, r.Ip, nil
				}
			}
		}
	}

	return -1, "0.0.0.0", errors.New("Cannot Find My NodeId and IP Address from VScrnDef json")
}

func (vdef *VScrnDef) resolveLifecyclePort(lifecyclePort int, legacyUclPort int) int {
	if lifecyclePort > 0 {
		return lifecyclePort
	}
	if legacyUclPort > 0 {
		return legacyUclPort
	}
	return DEFAULT_LIFECYCLE_PORT
}

func (vdef *VScrnDef) resolveLayoutPort(layoutPort int, legacyUlaPort int) int {
	if layoutPort > 0 {
		return layoutPort
	}
	if legacyUlaPort > 0 {
		return legacyUlaPort
	}
	return DEFAULT_LAYOUT_PORT
}

func (vdef *VScrnDef) GetLifecyclePort(nodeId int) (int, error) {

	for _, r := range vdef.DistributedWindowSystem.FrameworkNode {
		if nodeId == r.NodeId {
			return vdef.resolveLifecyclePort(r.LifecyclePort, r.Ucl.Port), nil
		}
	}

	return -1, errors.New("Cannnot Find My Port from VScrnDef json")
}

func (vdef *VScrnDef) GetLifecycleAddr() (string, error) {

	keyHostName, err := os.Hostname()
	if err != nil {
		return "", err
	}

	ipAddrs, err := GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		return "", err
	}

	nodeId, targetIp, err := vdef.GetNodeIdAndIpAddr(ipAddrs, keyHostName)
	if err != nil {
		return "", err
	}

	targetPort, err := vdef.GetLifecyclePort(nodeId)
	if err != nil {
		return "", err
	}

	targetAddr := targetIp + ":" + strconv.Itoa(targetPort)

	return targetAddr, nil

}

func (vdef *VScrnDef) GetFrameworkNode() ([]LauncherNode, error) {

	var dNodes []LauncherNode
	for _, node := range vdef.Nodes {
		for _, fwNode := range vdef.DistributedWindowSystem.FrameworkNode {
			if node.NodeId == fwNode.NodeId {
				var tmp LauncherNode
				tmp.HostName = node.HostName
				tmp.Ip = node.Ip
				tmp.Port = vdef.resolveLifecyclePort(fwNode.LifecyclePort, fwNode.Ucl.Port)
				dNodes = append(dNodes, tmp)
				break
			}
		}
	}

	if len(dNodes) > 0 {
		return dNodes, nil
	}

	return nil, errors.New("frameworkNode not found.")
}

func (vdef *VScrnDef) GetDistribNodes() ([]DistribNode, error) {

	var dNodes []DistribNode
	for _, node := range vdef.Nodes {
		for _, fwNode := range vdef.DistributedWindowSystem.FrameworkNode {
			if node.NodeId == fwNode.NodeId {
				var tmp DistribNode
				tmp.NodeId = node.NodeId
				tmp.Ip = node.Ip
				tmp.LayoutPort = vdef.resolveLayoutPort(fwNode.LayoutPort, fwNode.Ula.Port)
				dNodes = append(dNodes, tmp)
				break
			}
		}
	}

	if len(dNodes) > 0 {
		return dNodes, nil
	}

	return nil, errors.New("frameworkNode not found.")
}

func (vdef *VScrnDef) GetUhmiPort(nodeId int) (int, error) {
	if vdef.DistributedWindowSystem.UhmiMasterNode.Port > 0 && nodeId == vdef.DistributedWindowSystem.UhmiMasterNode.NodeId {
		return vdef.DistributedWindowSystem.UhmiMasterNode.Port, nil
	}

	if nodeId == vdef.DistributedWindowSystem.UhmiServer.NodeId {
		return vdef.DistributedWindowSystem.UhmiServer.Port, nil
	}

	return -1, errors.New("Cannnot Find My Port from VScrnDef json")
}

func (vdef *VScrnDef) GetUhmiServerNodeAddr() (string, error) {

	var targetAddr string
	masterNodeId := vdef.DistributedWindowSystem.UhmiMasterNode.NodeId
	masterPort := vdef.DistributedWindowSystem.UhmiMasterNode.Port
	legacyNodeId := vdef.DistributedWindowSystem.UhmiServer.NodeId
	legacyPort := vdef.DistributedWindowSystem.UhmiServer.Port

	for _, node := range vdef.Nodes {
		if masterPort > 0 && node.NodeId == masterNodeId {
			targetAddr = node.Ip + ":" + strconv.Itoa(masterPort)
			return targetAddr, nil
		}
	}

	for _, node := range vdef.Nodes {
		if legacyPort > 0 && node.NodeId == legacyNodeId {
			targetAddr = node.Ip + ":" + strconv.Itoa(legacyPort)
			return targetAddr, nil
		}
	}

	return targetAddr, errors.New("UhmiServer Node not found.")
}

func (vdef *VScrnDef) ReadAliasIp() (aip map[string]LauncherNode) {

	aip = make(map[string]LauncherNode)
	for _, r1 := range vdef.Nodes {
		var node LauncherNode
		node.Ip = r1.Ip
		node.HostName = r1.HostName
		for _, r2 := range vdef.DistributedWindowSystem.FrameworkNode {
			if r1.NodeId == r2.NodeId {
				node.Port = vdef.resolveLifecyclePort(r2.LifecyclePort, r2.Ucl.Port)
				break
			}
		}
		aip[r1.HostName] = node
	}

	return
}

func (vdef *VScrnDef) IsVDisplayInNode(nodeId int, vDisplayId int) bool {

	for _, rdisplay := range vdef.RealDisplays {
		if rdisplay.VDisplayId == vDisplayId && rdisplay.NodeId == nodeId {
			return true
		}
	}

	return false
}

func (vdef *VScrnDef) GetVDisplays() []layoutcore.VirtualDisplay {
	vDisplays := make([]layoutcore.VirtualDisplay, 0)
	for _, vdisplay := range vdef.Def2D.VirtualDisplays {
		var tmp layoutcore.VirtualDisplay
		tmp.DispName = vdisplay.DispName
		tmp.VDisplayId = vdisplay.VDisplayId
		tmp.VirtualX = vdisplay.VirtualX
		tmp.VirtualY = vdisplay.VirtualY
		tmp.VirtualW = vdisplay.VirtualW
		tmp.VirtualH = vdisplay.VirtualH
		vDisplays = append(vDisplays, tmp)
	}
	return vDisplays
}

func (vdef *VScrnDef) GetVDisplayAreas() []layoutcore.VirtualDisplayArea {
	areas := make([]layoutcore.VirtualDisplayArea, 0)
	for _, a := range vdef.Def2D.VirtualDisplayAreas {
		var tmp layoutcore.VirtualDisplayArea
		tmp.AreaName = a.AreaName
		tmp.VirtualX = a.VirtualX
		tmp.VirtualY = a.VirtualY
		tmp.VirtualW = a.VirtualW
		tmp.VirtualH = a.VirtualH
		areas = append(areas, tmp)
	}
	return areas
}

func (vdef *VScrnDef) GetLayoutPort(nodeId int) (int, error) {

	for _, r := range vdef.DistributedWindowSystem.FrameworkNode {
		if nodeId == r.NodeId {
			return vdef.resolveLayoutPort(r.LayoutPort, r.Ula.Port), nil
		}
	}

	return -1, errors.New("Cannot Find My Port from VScrnDef json")
}

// GetFrameworkNodeEnv returns framework node env variables.
// New format uses framework_node.env. Old format fallback is ucl_node.env.
func (vdef *VScrnDef) GetFrameworkNodeEnv(nodeId int) []string {
	for _, r := range vdef.DistributedWindowSystem.FrameworkNode {
		if nodeId != r.NodeId {
			continue
		}

		if r.Env != nil {
			return append([]string{}, (*r.Env)...)
		}

		if r.Ucl.Env != nil {
			return append([]string{}, (*r.Ucl.Env)...)
		}

		return nil
	}

	return nil
}

// GetRealDisplayByDispName retrieves real_display info from display_area (disp_name)
func (vdef *VScrnDef) GetRealDisplayByDispName(dispName string) (RealDisplay, error) {
	var vDisplayId int
	found := false
	for _, vdisplay := range vdef.Def2D.VirtualDisplays {
		if vdisplay.DispName == dispName {
			vDisplayId = vdisplay.VDisplayId
			found = true
			break
		}
	}
	if !found {
		return RealDisplay{}, errors.New("Display area '" + dispName + "' not found in virtual_screen_2d")
	}

	for _, rdisplay := range vdef.RealDisplays {
		if rdisplay.VDisplayId == vDisplayId {
			return rdisplay, nil
		}
	}

	return RealDisplay{}, errors.New("Real display not found for vdisplay_id: " + strconv.Itoa(vDisplayId))
}

// GetHostNameByNodeId retrieves hostname from node_id
func (vdef *VScrnDef) GetHostNameByNodeId(nodeId int) (string, error) {
	for _, node := range vdef.Nodes {
		if node.NodeId == nodeId {
			return node.HostName, nil
		}
	}
	return "", errors.New("Node not found for node_id: " + strconv.Itoa(nodeId))
}
