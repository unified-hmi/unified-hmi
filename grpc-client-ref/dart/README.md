# gRPC Client API for Dart

This directory provides a sample application for using the UHMI gRPC client
API in Dart.

API wrapper implementations are located in the `lib/uhmiapi/` directory.
Generate the gRPC protocol files into the `lib/src/generated/` directory
before building.

## 1. Generating Protocol Files

Before implementing gRPC client APIs, generate the necessary protocol files
for your development environment.

Prerequisites: `protoc` and the Dart protoc plugin.

```
dart pub global activate protoc_plugin
export PATH="$PATH:$HOME/.pub-cache/bin"
```

Generate the Dart stubs from `uhmi.proto` in this repository:

```
mkdir -p lib/src/generated
protoc --dart_out=grpc:lib/src/generated -I ../../proto ../../proto/uhmi.proto
```

Then install the dependencies:

```
dart pub get
```

## 2. Importing Generated Files

You can import all generated files at once using the wrapper file
`lib/uhmi_wrapper.dart`.

```dart
import 'package:uhmi_grpc_client/uhmi_wrapper.dart';
```

## 3. Implementing APIs in Your Applications

Reference implementations for the UHMI APIs are provided as functions in
`lib/uhmiapi/uhmi_api.dart`. You can easily use these APIs by importing them
into your application.

### Example

This example initializes a UHMI client and runs the ListDestinations command.

```dart
import 'package:uhmi_grpc_client/uhmiapi/uhmi_api.dart';

void main() async {
  final (client, channel) = await uhmiClientInit('127.0.0.1', 6643);
  await uhmiClientListDestinations(client);
  await channel.shutdown();
}
```

## 4. Sample Application

`bin/uhmi_grpc_client_ref.dart` is a command-line sample covering all
supported APIs. Please refer to it for more details.

```
dart run bin/uhmi_grpc_client_ref.dart -c ListDestinations
dart run bin/uhmi_grpc_client_ref.dart -c LaunchApp my-app dest-name -async
dart run bin/uhmi_grpc_client_ref.dart -c MoveWindow my-app dest-name -d 500 -curve 1
```

Run with `-h` to see the full command list.

**Note:** Before running these samples, you need to run the UHMI framework
(`uhmi-master-node` and worker nodes) and prepare the JSON configuration
files.
