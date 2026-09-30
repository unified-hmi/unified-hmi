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

import 'dart:io';

import 'package:grpc/grpc.dart';
import 'package:uhmi_grpc_client/uhmi_wrapper.dart';
import 'package:uhmi_grpc_client/uhmiapi/uhmi_api.dart';

/// printUsage prints instructions for the UHMI API command-line tool.
void printUsage() {
  print('''
Usage: uhmi_grpc_client_ref [options] [arguments...]

Options:
  -c <name>   UHMI API command to execute (default: ListDestinations)
  -s <ip>     server IP address (default: 127.0.0.1)
  -p <port>   server port (default: 6643)
  -async      LaunchApp: return without waiting for the app to exit
  -d <ms>     animation duration in milliseconds for MoveWindow(s)/WidenApp (default: 0)
  -curve <id> animation curve ID for MoveWindow(s)/WidenApp (default: 0)
  -h          show this message

Commands:
  StopAll                          no arguments
  ListAvailableApps                no arguments
  ListRunningApps                  no arguments
  GetAppStatus                     app_name
  StopApp                          app_name
  LaunchApp                        app_name [dest_name] [-async]
  LaunchApps                       app_name:dest_name ...
  MoveWindow                       app_name dest_name [-d ms] [-curve id]
  MoveWindows                      app_name:dest_name ... [-d ms] [-curve id]
  WidenApp                         app_name disp_name1 disp_name2 [-d ms] [-curve id]
  ListDestinations                 no arguments
  CreateAppMirror                  app_name dest_name
  RemoveAppMirror                  mirror_name
  ActivateWindow                   app_name
  DeactivateWindow                 app_name
  SetAppPriorityGroup              app_name group
  ListPriorityGroups               no arguments
''');
}

/// _needArgs checks the argument count and prints an error when it is short.
bool _needArgs(String command, List<String> args, int n, String usage) {
  if (args.length < n) {
    stderr.writeln('Error: $command requires $usage');
    return false;
  }
  return true;
}

/// _parseAppDest parses an "app_name:dest_name" argument. ":dest_name" may be omitted.
(String, String) _parseAppDest(String arg) {
  final idx = arg.indexOf(':');
  if (idx < 0) {
    return (arg, '');
  }
  return (arg.substring(0, idx), arg.substring(idx + 1));
}

Future<void> main(List<String> arguments) async {
  // Default command is ListDestinations
  var command = 'ListDestinations';
  var serverIp = '127.0.0.1';
  var serverPort = 6643;
  var asyncLaunch = false;
  var durationMs = 0.0;
  var curveId = 0;
  var showHelp = false;
  final args = <String>[];

  // Parse command-line arguments
  for (var i = 0; i < arguments.length; i++) {
    switch (arguments[i]) {
      case '-h':
        showHelp = true;
      case '-async':
        asyncLaunch = true;
      case '-c':
        if (i + 1 < arguments.length) command = arguments[++i];
      case '-s':
        if (i + 1 < arguments.length) serverIp = arguments[++i];
      case '-p':
        if (i + 1 < arguments.length) serverPort = int.parse(arguments[++i]);
      case '-d':
        if (i + 1 < arguments.length) durationMs = double.parse(arguments[++i]);
      case '-curve':
        if (i + 1 < arguments.length) curveId = int.parse(arguments[++i]);
      default:
        args.add(arguments[i]);
    }
  }

  // If help argument is set, print usage and exit
  if (showHelp) {
    printUsage();
    exit(0);
  }

  // Initialize gRPC client for UHMI API
  final (client, channel) = await uhmiClientInit(serverIp, serverPort);

  try {
    // Execute the specified command
    switch (command) {
      case 'StopAll':
        await uhmiClientStopAll(client);
      case 'ListAvailableApps':
        await uhmiClientListAvailableApps(client);
      case 'ListRunningApps':
        await uhmiClientListRunningApps(client);
      case 'GetAppStatus':
        if (!_needArgs(command, args, 1, 'app_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientGetAppStatus(client, args[0]);
      case 'StopApp':
        if (!_needArgs(command, args, 1, 'app_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientStopApp(client, args[0]);
      case 'LaunchApp':
        if (!_needArgs(command, args, 1, 'app_name [dest_name]')) {
          exitCode = 1;
          break;
        }
        await uhmiClientLaunchApp(client, args[0],
            destName: args.length >= 2 ? args[1] : '', asyncLaunch: asyncLaunch);
      case 'LaunchApps':
        if (!_needArgs(command, args, 1, 'app_name:dest_name ...')) {
          exitCode = 1;
          break;
        }
        final apps = args.map((arg) {
          final (appName, destName) = _parseAppDest(arg);
          return LaunchAppEntry(appName: appName, destName: destName);
        }).toList();
        await uhmiClientLaunchApps(client, apps);
      case 'MoveWindow':
        if (!_needArgs(command, args, 2, 'app_name dest_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientMoveWindow(client, args[0], args[1],
            durationMs: durationMs, curveId: curveId);
      case 'MoveWindows':
        if (!_needArgs(command, args, 1, 'app_name:dest_name ...')) {
          exitCode = 1;
          break;
        }
        final windows = args.map((arg) {
          final (appName, destName) = _parseAppDest(arg);
          return MoveWindowEntry(
              appName: appName, destName: destName, durationMs: durationMs, curveId: curveId);
        }).toList();
        await uhmiClientMoveWindows(client, windows);
      case 'WidenApp':
        if (!_needArgs(command, args, 3, 'app_name disp_name1 disp_name2')) {
          exitCode = 1;
          break;
        }
        await uhmiClientWidenApp(client, args[0], args[1], args[2],
            durationMs: durationMs, curveId: curveId);
      case 'ListDestinations':
        await uhmiClientListDestinations(client);
      case 'CreateAppMirror':
        if (!_needArgs(command, args, 2, 'app_name dest_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientCreateAppMirror(client, args[0], args[1]);
      case 'RemoveAppMirror':
        if (!_needArgs(command, args, 1, 'mirror_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientRemoveAppMirror(client, args[0]);
      case 'ActivateWindow':
        if (!_needArgs(command, args, 1, 'app_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientActivateWindow(client, args[0]);
      case 'DeactivateWindow':
        if (!_needArgs(command, args, 1, 'app_name')) {
          exitCode = 1;
          break;
        }
        await uhmiClientDeactivateWindow(client, args[0]);
      case 'SetAppPriorityGroup':
        if (!_needArgs(command, args, 2, 'app_name group')) {
          exitCode = 1;
          break;
        }
        await uhmiClientSetAppPriorityGroup(client, args[0], args[1]);
      case 'ListPriorityGroups':
        await uhmiClientListPriorityGroups(client);
      default:
        stderr.writeln('Error: unknown command "$command"');
        printUsage();
        exitCode = 1;
    }
  } on GrpcError catch (e) {
    stderr.writeln('gRPC error: ${e.codeName} ${e.message}');
    exitCode = 1;
  } finally {
    await channel.shutdown();
  }
}
