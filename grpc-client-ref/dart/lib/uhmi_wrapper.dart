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

/// Wrapper library that re-exports the protoc generated UHMI stubs at once.
///
/// Generate the stubs into `lib/src/generated/` first:
/// `protoc --dart_out=grpc:lib/src/generated -I ../../proto ../../proto/uhmi.proto`
library;

export 'src/generated/uhmi.pb.dart';
export 'src/generated/uhmi.pbenum.dart';
export 'src/generated/uhmi.pbgrpc.dart';
export 'src/generated/uhmi.pbjson.dart';
