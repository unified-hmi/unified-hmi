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

package lifecycle

import (
	"errors"
	"fmt"
	"net"
	"time"
	"unified-hmi/internal/config"
)

func ReadCommand(conn net.Conn) ([]byte, []byte, error) {

	commType, err := ConnReadWithSize(conn)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("read command type err"))
	}

	command, err := ConnReadWithSize(conn)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("read command data err"))
	}

	return commType, command, nil
}

func SendCommand(conn net.Conn, commType string, command string) error {

	err := ConnWriteWithSize(conn, []byte(commType))
	if err != nil {
		return err
	}

	err = ConnWriteWithSize(conn, []byte(command))
	if err != nil {
		return err
	}

	return nil
}

func ReadStatus(conn net.Conn) ([]byte, error) {

	recvStatus, err := ConnReadWithSize(conn)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("read status err"))
	}

	return recvStatus, nil
}

func ResponseStatus(conn net.Conn, status string) error {

	err := ConnWriteWithSize(conn, []byte(status))
	if err != nil {
		return err
	}

	return nil
}

func ConnWrite(conn net.Conn, message []byte) error {
	return config.WriteFrame(conn, message)
}

func ConnWriteWithSize(conn net.Conn, message []byte) error {
	return config.WriteFrame(conn, message)
}

func ConnReadLoop(conn net.Conn, rcvChan chan []byte) {
	config.ReadFrameLoop(conn, rcvChan, MaxCommandSize)
}

// MaxCommandSize is the maximum accepted lifecycle command frame size.
const MaxCommandSize = 0x10000

func ConnReadWithSize(conn net.Conn) ([]byte, error) {
	buf, err := config.ReadFrame(conn, MaxCommandSize)
	if err != nil {
		return nil, fmt.Errorf("Command Read Fail: %w", err)
	}
	return buf, nil
}

func ConnectTarget(addr string) (net.Conn, error) {
	return config.DialWithTimeout("tcp", addr, 1*time.Second, 10*time.Millisecond)
}
