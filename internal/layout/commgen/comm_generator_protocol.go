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

package layoutcommgen

// UPIVsurface is the LAYOUT protocol message for an initial vsurface.
type UPIVsurface struct {
	AppName     string  `json:"appli_name"`
	VID         int     `json:"VID"`
	PixelW      float64 `json:"pixel_w"`
	PixelH      float64 `json:"pixel_h"`
	PsrcX       float64 `json:"psrc_x"`
	PsrcY       float64 `json:"psrc_y"`
	PsrcW       float64 `json:"psrc_w"`
	PsrcH       float64 `json:"psrc_h"`
	VdstX       float64 `json:"vdst_x"`
	VdstY       float64 `json:"vdst_y"`
	VdstW       float64 `json:"vdst_w"`
	VdstH       float64 `json:"vdst_h"`
	Visibility  *int    `json:"visibility"`
	WlSurfaceId int     `json:"wl_surface_id,omitempty"`
}

// UPIVlayer is the LAYOUT protocol message for an initial vlayer.
type UPIVlayer struct {
	AppName    string        `json:"appli_name"`
	VID        int           `json:"VID"`
	Coord      string        `json:"coord"`
	VdisplayId int           `json:"vdisplay_id"`
	VirtualW   float64       `json:"virtual_w"`
	VirtualH   float64       `json:"virtual_h"`
	VsrcX      float64       `json:"vsrc_x"`
	VsrcY      float64       `json:"vsrc_y"`
	VsrcW      float64       `json:"vsrc_w"`
	VsrcH      float64       `json:"vsrc_h"`
	VdstX      float64       `json:"vdst_x"`
	VdstY      float64       `json:"vdst_y"`
	VdstW      float64       `json:"vdst_w"`
	VdstH      float64       `json:"vdst_h"`
	Visibility *int          `json:"visibility"`
	Surface    []UPIVsurface `json:"vsurface"`
}

// UPIVscreen is the LAYOUT protocol message for an initial vscreen.
type UPIVscreen struct {
	Command      string        `json:"command"`
	InsertOrder  string        `json:"insert_order"`
	ReferenceVID int           `json:"referenceVID"`
	ParentVID    int           `json:"ParentVID"`
	Layer        []UPIVlayer   `json:"vlayer"`
	Surface      []UPIVsurface `json:"vsurface"`
}
