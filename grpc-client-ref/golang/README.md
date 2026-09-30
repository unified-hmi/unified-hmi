# gRPC Client API for Go

This directory provides a sample application for using the UHMI gRPC client
API in Go.

API wrapper implementations are located in the `uhmiapi/` directory. Generate
the gRPC protocol files into the `proto/` directory before building.

## 1. Generating Protocol Files

Before implementing gRPC client APIs, generate the necessary protocol files
for your development environment.

Prerequisites: `protoc`, `protoc-gen-go` and `protoc-gen-go-grpc`.

```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Copy `uhmi.proto` from this repository and generate the Go stubs:

```
mkdir -p proto
cp ../../proto/uhmi.proto proto/
protoc --go_out=proto --go-grpc_out=proto proto/uhmi.proto
```

(The same files are generated from the repository root with `make proto`.)

Then resolve the module dependencies:

```
go mod tidy
```

## 2. Importing Generated Files

Import the generated files to use the gRPC APIs.

```
import "uhmi-client/proto/grpc/uhmi"
```

## 3. Implementing APIs in Your Applications

Reference implementations for the UHMI APIs are provided as functions in the
`uhmiapi` package. You can easily use these APIs by importing them into your
application.

### Example

This example initializes a UHMI client and runs the ListDestinations command.

```go
package main

import (
	"context"
	"time"

	"uhmi-client/uhmiapi"
)

func main() {
	conn, client, _ := uhmiapi.UhmiClientInit("127.0.0.1", 6643)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer conn.Close()
	defer cancel()

	uhmiapi.UhmiClientListDestinations(client, ctx)
}
```

## 4. Sample Application

`uhmi_grpc_client_ref.go` is a command-line sample covering all supported
APIs. Please refer to it for more details.

```
go run uhmi_grpc_client_ref.go -c ListDestinations
go run uhmi_grpc_client_ref.go -c LaunchApp my-app dest-name -async
go run uhmi_grpc_client_ref.go -c MoveWindow my-app dest-name -d 500 -curve 1
```

Run with `-h` to see the full command list.

**Note:** Before running these samples, you need to run the UHMI framework
(`uhmi-master-node` and worker nodes) and prepare the JSON configuration
files.
