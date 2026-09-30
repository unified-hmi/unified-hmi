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

import 'package:grpc/grpc.dart';
import 'package:uhmi_grpc_client/uhmi_wrapper.dart';

/// uhmiClientInit initializes a gRPC client connection to the UHMI service.
/// The returned [ClientChannel] must be shut down by the caller.
Future<(UHMIServiceClient, ClientChannel)> uhmiClientInit(
    String serverIp, int serverPort) async {
  // Create a gRPC channel to connect to the server
  final channel = ClientChannel(
    serverIp,
    port: serverPort,
    options: const ChannelOptions(credentials: ChannelCredentials.insecure()),
  );

  // Return a UHMI service client using the channel
  return (UHMIServiceClient(channel), channel);
}

/// _printResponse prints the common status/info pair of a UHMI response.
void _printResponse(String apiName, String status, String info) {
  if (info.isNotEmpty) {
    print('$apiName response: status=$status info=$info');
  } else {
    print('$apiName response: status=$status');
  }
}

/// uhmiClientStopAll stops the whole UHMI system.
Future<void> uhmiClientStopAll(UHMIServiceClient client) async {
  final response = await client.stopAll(Empty());
  _printResponse('StopAll', response.status, response.info);
}

/// uhmiClientListAvailableApps lists the applications that can be launched.
Future<void> uhmiClientListAvailableApps(UHMIServiceClient client) async {
  final response = await client.listAvailableApps(Empty());
  _printResponse('ListAvailableApps', response.status, response.info);
}

/// uhmiClientListRunningApps lists the currently running applications.
Future<void> uhmiClientListRunningApps(UHMIServiceClient client) async {
  final response = await client.listRunningApps(Empty());
  _printResponse('ListRunningApps', response.status, response.info);
}

/// uhmiClientGetAppStatus gets the status of the specified application.
Future<void> uhmiClientGetAppStatus(UHMIServiceClient client, String appName) async {
  final request = AppControlRequest(appName: appName);
  final response = await client.getAppStatus(request);
  _printResponse('GetAppStatus', response.status, response.info);
}

/// uhmiClientStopApp stops the specified application.
Future<void> uhmiClientStopApp(UHMIServiceClient client, String appName) async {
  final request = AppControlRequest(appName: appName);
  final response = await client.stopApp(request);
  _printResponse('StopApp', response.status, response.info);
}

/// uhmiClientLaunchApp launches an application on the specified destination.
/// When [destName] is empty, the initial layout defined in initial_layout.json
/// is used. When [asyncLaunch] is true, the call returns right after the
/// layout is applied without waiting for the application to exit.
Future<void> uhmiClientLaunchApp(UHMIServiceClient client, String appName,
    {String destName = '', bool asyncLaunch = false}) async {
  final request = LaunchAppRequest(appName: appName, destName: destName, async_: asyncLaunch);
  final response = await client.launchApp(request);
  _printResponse('LaunchApp', response.status, response.info);
}

/// uhmiClientLaunchApps launches multiple applications simultaneously (best-effort).
Future<void> uhmiClientLaunchApps(UHMIServiceClient client, List<LaunchAppEntry> apps) async {
  final request = LaunchAppsRequest(apps: apps);
  final response = await client.launchApps(request);
  _printResponse('LaunchApps', response.status, response.info);
  for (final result in response.results) {
    print('  ${result.appName}: status=${result.status} info=${result.info}');
  }
}

/// uhmiClientMoveWindow moves the window of the specified application to [destName].
Future<void> uhmiClientMoveWindow(UHMIServiceClient client, String appName, String destName,
    {double durationMs = 0, int curveId = 0}) async {
  final request = MoveWindowRequest(
      appName: appName, destName: destName, durationMs: durationMs, curveId: curveId);
  final response = await client.moveWindow(request);
  _printResponse('MoveWindow', response.status, response.info);
}

/// uhmiClientMoveWindows moves multiple application windows simultaneously.
Future<void> uhmiClientMoveWindows(UHMIServiceClient client, List<MoveWindowEntry> windows) async {
  final request = MoveWindowsRequest(windows: windows);
  final response = await client.moveWindows(request);
  _printResponse('MoveWindows', response.status, response.info);
  for (final result in response.results) {
    print('  ${result.appName}: status=${result.status} info=${result.info}');
  }
}

/// uhmiClientWidenApp widens an application window across two adjacent virtual displays.
Future<void> uhmiClientWidenApp(
    UHMIServiceClient client, String appName, String dispName1, String dispName2,
    {double durationMs = 0, int curveId = 0}) async {
  final request = WidenAppRequest(
      appName: appName,
      dispName1: dispName1,
      dispName2: dispName2,
      durationMs: durationMs,
      curveId: curveId);
  final response = await client.widenApp(request);
  _printResponse('WidenApp', response.status, response.info);
}

/// uhmiClientListDestinations enumerates all valid dest_name values.
Future<void> uhmiClientListDestinations(UHMIServiceClient client) async {
  final response = await client.listDestinations(ListDestinationsRequest());
  _printResponse('ListDestinations', response.status, response.info);
  for (final dest in response.destinations) {
    print('  [tier ${dest.tier}] ${dest.name}: '
        '(${dest.virtualX}, ${dest.virtualY}) ${dest.virtualW}x${dest.virtualH}');
  }
}

/// uhmiClientCreateAppMirror mirrors a running application to another destination.
Future<void> uhmiClientCreateAppMirror(UHMIServiceClient client, String appName, String destName) async {
  final request = CreateAppMirrorRequest(appName: appName, destName: destName);
  final response = await client.createAppMirror(request);
  _printResponse('CreateAppMirror', response.status, response.info);
  if (response.mirrorName.isNotEmpty) {
    print('  mirror_name: ${response.mirrorName}');
  }
}

/// uhmiClientRemoveAppMirror removes a previously created mirror layer.
Future<void> uhmiClientRemoveAppMirror(UHMIServiceClient client, String mirrorName) async {
  final request = RemoveAppMirrorRequest(mirrorName: mirrorName);
  final response = await client.removeAppMirror(request);
  _printResponse('RemoveAppMirror', response.status, response.info);
}

/// uhmiClientActivateWindow brings the application layers to the front of their priority group.
Future<void> uhmiClientActivateWindow(UHMIServiceClient client, String appName) async {
  final request = ActivateWindowRequest(appName: appName);
  final response = await client.activateWindow(request);
  _printResponse('ActivateWindow', response.status, response.info);
}

/// uhmiClientDeactivateWindow sends the application layers to the back of their
/// priority group and hides them.
Future<void> uhmiClientDeactivateWindow(UHMIServiceClient client, String appName) async {
  final request = DeactivateWindowRequest(appName: appName);
  final response = await client.deactivateWindow(request);
  _printResponse('DeactivateWindow', response.status, response.info);
}

/// uhmiClientSetAppPriorityGroup reassigns all layers of an application to the
/// specified priority group.
Future<void> uhmiClientSetAppPriorityGroup(
    UHMIServiceClient client, String appName, String group) async {
  final request = SetAppPriorityGroupRequest(appName: appName, group: group);
  final response = await client.setAppPriorityGroup(request);
  _printResponse('SetAppPriorityGroup', response.status, response.info);
}

/// uhmiClientListPriorityGroups lists all priority groups with their runtime
/// app assignments.
Future<void> uhmiClientListPriorityGroups(UHMIServiceClient client) async {
  final response = await client.listPriorityGroups(Empty());
  _printResponse('ListPriorityGroups', response.status, response.info);
  for (final group in response.groups) {
    print('  ${group.name} (priority=${group.priority}, default=${group.isDefault}):');
    for (final layer in group.layers) {
      print('    ${layer.appName}/${layer.areaName}: vid=${layer.vid} '
          'z_rank=${layer.zRank} visible=${layer.visible} '
          'dst=(${layer.dstX}, ${layer.dstY}, ${layer.dstW}, ${layer.dstH})');
    }
  }
}
