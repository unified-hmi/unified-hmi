# UHMI gRPC Client Reference

This directory provides minimal reference implementations of gRPC clients for
UHMI (Unified HMI) in multiple programming languages (Go and Dart).

It is intended to help developers understand how to call the gRPC APIs defined
in [proto/uhmi.proto](../proto/uhmi.proto) from their own applications.

## Overview

All UHMI APIs are exposed through a single gRPC service (`UHMIService`) on the
UHMI master node (`uhmi-master-node`). Once the UHMI framework is running, you
can connect to the service from any application or command line and use its
functions.

The server address is configured in `virtual-screen-def.json`:

```json
"distributed_window_system": {
    "uhmi_master_node": {
        "node_id": 1,
        "port": 6643
    }
}
```

Connect to `<IP of node 1>:6643` (see the `node` section for the IP address
of each node ID). The samples default to `127.0.0.1:6643`.

## Covered APIs

To keep the samples minimal, only the following 17 RPCs are covered. Other
RPCs defined in `uhmi.proto` are intentionally out of scope.

- System orchestration: `StopAll`
- Application lifecycle: `ListAvailableApps`, `ListRunningApps`, `GetAppStatus`, `StopApp`
- App launch & placement: `LaunchApp`, `LaunchApps`, `MoveWindow`, `MoveWindows`, `WidenApp`, `ListDestinations`
- App mirroring: `CreateAppMirror`, `RemoveAppMirror`
- Window activation & priority groups: `ActivateWindow`, `DeactivateWindow`, `SetAppPriorityGroup`, `ListPriorityGroups`

## Prerequisites

Before running the gRPC clients, ensure that the UHMI framework
(`uhmi-master-node` and `uhmi-worker-node`) is running with the required JSON configuration files. See the top-level [README](../README.md) for setup.

## Protocol file

The samples use the protocol file in this repository:

- [proto/uhmi.proto](../proto/uhmi.proto)

Each language directory explains how to generate client stubs from it.

## Languages

- [golang/](golang/README.md) — Go reference implementation
- [dart/](dart/README.md) — Dart reference implementation
