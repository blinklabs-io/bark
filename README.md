# Bark Protocol

Bark is a gRPC-based communication protocol for [Dingo](https://github.com/blinklabs-io/dingo) blockchain nodes. This repository contains the protocol buffer definitions and generated Go code.

## Overview

Bark enables Dingo-to-Dingo communication for managing and operating Dingo instances. The protocol is designed to be extensible, supporting various operational aspects as they become needed.

Current protocols:
- **Archive Proxy**: efficient retrieval of immutable blockchain data via signed URLs to cloud object storage (S3, GCS, etc.)
- **Database Service**: database lifecycle management: snapshots, restoration, and chain truncation with async progress streaming
- **Lifecycle Service**: remote process-lifecycle control: graceful stop, graceful restart, and status reporting
- **Event Service**: subscribe to and publish a node's EventBus events over the network, with an at-least-once variant that resumes after a disconnect

## Features

- **Dingo-to-Dingo Communication**: Protocol for managing and operating Dingo instances
- **Extensible Design**: Modular protocol structure supporting future operational needs
- **gRPC Protocol**: Strong contracts with Protocol Buffers
- **ConnectRPC**: Modern RPC framework for Go
- **Buf Integration**: Automated code generation and linting
- **v1alpha1**: Alpha release allowing for rapid iteration

## Repository Structure

```plaintext
proto/v1alpha1/
  archive/                           - Archive Proxy protocol
    archive.proto                    - Protocol buffer definition
    archive.pb.go                    - Generated protobuf Go code
    archivev1alpha1connect/          - Generated ConnectRPC service code
  database/                          - Database Service protocol
    database.proto                   - Protocol buffer definition
    database.pb.go                   - Generated protobuf Go code
    databasev1alpha1connect/         - Generated ConnectRPC service code
  lifecycle/                         - Lifecycle Service protocol
    lifecycle.proto                  - Protocol buffer definition
    lifecycle.pb.go                  - Generated protobuf Go code
    lifecyclev1alpha1connect/        - Generated ConnectRPC service code
  event/                             - Event Service protocol
    event.proto                      - Protocol buffer definition
    event.pb.go                      - Generated protobuf Go code
    eventv1alpha1connect/            - Generated ConnectRPC service code
```

## Usage

Import the generated Go code in your project:

```go
import (
    archivev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/archive"
    "github.com/blinklabs-io/bark/proto/v1alpha1/archive/archivev1alpha1connect"

    databasev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/database"
    "github.com/blinklabs-io/bark/proto/v1alpha1/database/databasev1alpha1connect"

    lifecyclev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/lifecycle"
    "github.com/blinklabs-io/bark/proto/v1alpha1/lifecycle/lifecyclev1alpha1connect"
)
```

## Development

### Prerequisites

- Go 1.26.10 or newer
- make and Python 3 (`make generate` runs a Python script)
- [buf](https://buf.build) CLI, golines, nilaway, and golangci-lint, installed
  by `make install-tools`

### Setup

```bash
# Clone the repository
git clone https://github.com/blinklabs-io/bark.git
cd bark

# Install dependencies
go mod download

# Install buf, golines, nilaway, and golangci-lint into the Go bin directory
# and put that directory on PATH
make install-tools
export PATH="$PATH:$(go env GOPATH)/bin"

# Generate code from proto files
make generate
```

### Common Commands

```bash
# Format proto and Go files
make format

# Lint proto and Go code
make lint

# Build Go code
make build

# Run tests
make test

# Run the checks CI runs
make check

# List every target
make help
```

## Protocol

### Archive Proxy (v1alpha1)

Service for retrieving immutable block data via signed cloud storage URLs.

**RPC Methods**: `FetchBlock`

### Database Service (v1alpha1)

Service for database lifecycle management with mutual exclusion enforcement.

**RPC Methods**: `CreateSnapshot`, `GetSnapshotStatus`, `ListSnapshots`, `DeleteSnapshot`, `VerifySnapshot`, `Restore`, `GetRestoreStatus`, `ListAvailableSnapshots`, `Truncate`, `GetTruncateStatus`, `StreamOperationProgress`, `GetOperationHistory`, `GetDatabaseInfo`, `CancelOperation`

### Lifecycle Service (v1alpha1)

Service for remote process-lifecycle control: graceful stop, graceful restart (stop + re-execution), and status reporting. Requires mTLS-authenticated callers; only one of Stop or Restart may run at a time.

**RPC Methods**: `Stop`, `Restart`, `GetStatus`

### Event Service (v1alpha1)

Service that exposes a node's EventBus over the network. `Subscribe` is at-most-once; `SubscribeWithAck` is at-least-once and resumes from the last acknowledged sequence after a disconnect. `Publish` writes into the node's EventBus, so a server may leave it disabled.

**RPC Methods**: `Subscribe`, `SubscribeWithAck`, `Publish`, `ListEventTypes`

The `eventtype` package lists the event types a node exposes, and the `eventclient` package wraps the Event Service streams: `SubscribeWithAck` reconnects and resumes from the last acknowledged sequence on its own.

See [PROTOCOL_DESIGN.md](PROTOCOL_DESIGN.md) for detailed protocol documentation.

## Contributing

This project uses:
- Conventional commits for commit messages
- Weekly automated dependency updates via Dependabot
- CI/CD with GitHub Actions for linting and testing

## Documentation

- [PROTOCOL_DESIGN.md](PROTOCOL_DESIGN.md) - Detailed protocol design and architecture
- [Dingo Issue #335](https://github.com/blinklabs-io/dingo/issues/335) - Original requirements

## License

Apache-2.0 - see [LICENSE](LICENSE) file for details

## Related Projects

- [Dingo](https://github.com/blinklabs-io/dingo) - Cardano blockchain node implementation
- [utxorpc/spec](https://github.com/utxorpc/spec) - Reference for protocol organization
