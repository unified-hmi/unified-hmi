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

// This file centralizes the raw-TCP communication primitives shared by the
// uhmi master node and worker nodes: length-prefixed framing, retried dialing,
// and the send/receive pump loop. It intentionally depends only on the
// standard library so that both the layout (multiconn) and lifecycle packages
// can use it without creating an import cycle.
package config

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const frameHeaderSize = 4

// WriteFrame writes message to w as a single frame: a 4-byte big-endian
// length prefix followed by the payload. Partial writes are completed by
// looping until the whole frame has been written.
func WriteFrame(w io.Writer, message []byte) error {
	szBuf := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(szBuf, uint32(len(message)))
	if err := writeFull(w, szBuf); err != nil {
		return err
	}
	if len(message) == 0 {
		return nil
	}
	return writeFull(w, message)
}

func writeFull(w io.Writer, buf []byte) error {
	for total := 0; total < len(buf); {
		n, err := w.Write(buf[total:])
		if err != nil {
			return fmt.Errorf("write error: %w", err)
		}
		if n == 0 {
			return errors.New("write error: wrote 0 bytes")
		}
		total += n
	}
	return nil
}

// ReadFrame reads one frame from r: a 4-byte big-endian length prefix
// followed by that many payload bytes.
//
// A clean EOF before the first header byte is returned as io.EOF so callers
// can distinguish an orderly close from a truncated stream. A zero length
// prefix is meaningless and reported as an error. When maxSize is non-zero,
// a length prefix larger than maxSize is rejected before any payload is
// read.
func ReadFrame(r io.Reader, maxSize uint32) ([]byte, error) {
	szBuf := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(r, szBuf); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("frame size read fail: %w", err)
	}

	recvSize := binary.BigEndian.Uint32(szBuf)
	if recvSize == 0 {
		return nil, errors.New("zero byte read(meaningless)")
	}
	if maxSize > 0 && recvSize > maxSize {
		return nil, fmt.Errorf("frame size %d exceeds max %d", recvSize, maxSize)
	}

	recvBuf := make([]byte, recvSize)
	if _, err := io.ReadFull(r, recvBuf); err != nil {
		return nil, fmt.Errorf("frame read fail: %w", err)
	}
	return recvBuf, nil
}

// ReadFrameLoop reads frames from conn and pushes them onto rcvChan. On any
// read error (including the peer closing the connection) it pushes a single
// nil to signal termination and returns.
func ReadFrameLoop(conn net.Conn, rcvChan chan<- []byte, maxSize uint32) {
	for {
		buf, err := ReadFrame(conn, maxSize)
		if err != nil {
			rcvChan <- nil
			return
		}
		rcvChan <- buf
	}
}

// DialWithRetry dials network/addr, retrying every interval until it
// succeeds or ctx is done. The first attempt is made immediately. Each
// attempt uses DialContext so that ctx also interrupts an in-progress
// dial (e.g. a black-holed address stuck in the OS TCP connect timeout).
func DialWithRetry(ctx context.Context, network, addr string, interval time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{}
	for {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("dial %s %s: %w", network, addr, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// DialWithTimeout dials network/addr, retrying every interval until timeout
// elapses. With timeout <= 0 only a single attempt is made.
func DialWithTimeout(network, addr string, timeout, interval time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		conn, err := net.Dial(network, addr)
		if err != nil {
			return nil, fmt.Errorf("dial %s %s: %w", network, addr, err)
		}
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return DialWithRetry(ctx, network, addr, interval)
}

// ProbeAddr reports whether a single dial to network/addr succeeds within
// timeout. The probing connection is closed immediately.
func ProbeAddr(network, addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout(network, addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// RequestResponse connects to network/addr, writes every frame in frames,
// reads one response frame, closes the connection, and returns the response
// payload. timeout bounds the initial dial (retried at interval when
// positive); maxResp bounds the accepted response size (0 = unlimited).
func RequestResponse(network, addr string, timeout, interval time.Duration, frames [][]byte, maxResp uint32) ([]byte, error) {
	conn, err := DialWithTimeout(network, addr, timeout, interval)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	for _, frame := range frames {
		if err := WriteFrame(conn, frame); err != nil {
			return nil, err
		}
	}
	return ReadFrame(conn, maxResp)
}

// PumpHandlers configures FramePump. All callbacks are optional; a nil
// callback selects the default behavior described per field.
type PumpHandlers struct {
	// MaxFrame bounds accepted incoming frame sizes (0 = unlimited).
	MaxFrame uint32
	// TransformSend rewrites each outgoing message before it is written.
	// Returning nil (or leaving TransformSend nil) writes the message
	// unchanged.
	TransformSend func(msg []byte) []byte
	// OnMessage is called for each received frame. Returning false stops the
	// pump. Nil discards received frames and keeps pumping.
	OnMessage func(msg []byte) (cont bool)
	// OnClosed is called once when the peer closes the connection (or a read
	// error occurs), just before the pump returns.
	OnClosed func()
	// OnSendError is called when writing an outgoing frame fails, just before
	// the pump returns.
	OnSendError func(err error)
}

// FramePump pumps messages between sendCh and conn until ctx is done, the
// peer closes the connection, a write fails, or OnMessage requests a stop.
// It spawns a ReadFrameLoop goroutine and returns its channel so the caller
// can keep draining in-flight messages after the pump returns (e.g. to
// observe the peer's close acknowledgement).
func FramePump(ctx context.Context, conn net.Conn, sendCh <-chan []byte, h PumpHandlers) (rcvCh <-chan []byte) {
	rcv := make(chan []byte, 2)
	go ReadFrameLoop(conn, rcv, h.MaxFrame)

	for {
		select {
		case sendMsg := <-sendCh:
			if h.TransformSend != nil {
				if tx := h.TransformSend(sendMsg); tx != nil {
					sendMsg = tx
				}
			}
			if err := WriteFrame(conn, sendMsg); err != nil {
				if h.OnSendError != nil {
					h.OnSendError(err)
				}
				return rcv
			}
		case recvMsg := <-rcv:
			if recvMsg == nil {
				if h.OnClosed != nil {
					h.OnClosed()
				}
				return rcv
			}
			if h.OnMessage != nil && !h.OnMessage(recvMsg) {
				return rcv
			}
		case <-ctx.Done():
			return rcv
		}
	}
}
