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

package layoutcore

// should be -1 if SurfaceId is not used
type IdPair struct {
	LayerId   int `json:"LayerId"`
	SurfaceId int `json:"SurfaceId"`
}

type ApplyCommandData struct {
	Command   string            `json:"Command"`
	ChgIds    []IdPair          `json:"ChgIds"`
	NPScreens *NodePixelScreens `json:"NPScreens"`
}

type NodePixelScreens struct {
	NodeId   int           `json:"NodeId"`
	Pscreens []PixelScreen `json:"Pscreens"`
}

type PixelScreen struct {
	Rdisplay     RealDisplay       `json:"Rdisplay"`
	Players      []PixelLayer      `json:"Players"`
	PsafetyAreas []PixelSafetyArea `json:"PsafetyAreas"`
}

type RealDisplay struct {
	NodeId     int `json:"NodeId"`
	PixelW     int `json:"PixelW"`
	PixelH     int `json:"PixelH"`
	VDisplayId int `json:"VDisplayId"`
	RDisplayId int `json:"RDisplayId"`
}

type PixelLayer struct {
	AppName string `json:"AppName"`

	VID    int     `json:"VID"`
	PixelW float64 `json:"PixelW"`
	PixelH float64 `json:"PixelH"`

	PsrcX float64 `json:"PsrcX"`
	PsrcY float64 `json:"PsrcY"`
	PsrcW float64 `json:"PsrcW"`
	PsrcH float64 `json:"PsrcH"`

	PdstX float64 `json:"PdstX"`
	PdstY float64 `json:"PdstY"`
	PdstW float64 `json:"PdstW"`
	PdstH float64 `json:"PdstH"`

	Visibility int `json:"Visibility"`

	Psurfaces []PixelSurface `json:"Psurfaces"`
}

type PixelSurface struct {
	AppName string `json:"AppName"`

	ParentVID int `json:"ParentVID"`
	VID       int `json:"VID"`

	PixelW float64 `json:"PixelW"`
	PixelH float64 `json:"PixelH"`

	PsrcX float64 `json:"PsrcX"`
	PsrcY float64 `json:"PsrcY"`
	PsrcW float64 `json:"PsrcW"`
	PsrcH float64 `json:"PsrcH"`

	PdstX float64 `json:"PdstX"`
	PdstY float64 `json:"PdstY"`
	PdstW float64 `json:"PdstW"`
	PdstH float64 `json:"PdstH"`

	Visibility  int `json:"Visibility"`
	WlSurfaceId int `json:"WlSurfaceId"`
}

type PixelSafetyArea struct {
	PixelX float64 `json:"PixelX"`
	PixelY float64 `json:"PixelY"`
	PixelW float64 `json:"PixelW"`
	PixelH float64 `json:"PixelH"`
}
