# Bark Protocol

Bark is a gRPC-based communication protocol for [Dingo](https://github.com/blinklabs-io/dingo) blockchain nodes. This repository contains the protocol buffer definitions and generated Go code.

## Overview

Bark enables Dingo-to-Dingo communication for managing and operating Dingo instances. The protocol is designed to be extensible, supporting various operational aspects as they become needed.

Current protocols:
- **Archive Proxy**: efficient retrieval of immutable blockchain data via signed URLs to cloud object storage (S3, GCS, etc.)
- **Database Service**: database lifecycle management: snapshots, restoration, and chain truncation with async progress streaming

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
```

## Usage

Import the generated Go code in your project:

```go
import (
    archivev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/archive"
    "github.com/blinklabs-io/bark/proto/v1alpha1/archive/archivev1alpha1connect"

    databasev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/database"
    "github.com/blinklabs-io/bark/proto/v1alpha1/database/databasev1alpha1connect"
)
```

## Development

### Prerequisites

- Go 1.24+
- [buf](https://buf.build) CLI

### Setup

```bash
# Clone the repository
git clone https://github.com/blinklabs-io/bark.git
cd bark

# Install dependencies
go mod download

# Generate code from proto files
buf generate
```

### Common Commands

```bash
# Format proto files
buf format -w

# Lint proto files
buf lint

# Build Go code
go build ./...

# Run tests
go test ./...
```

## Protocol

### Archive Proxy (v1alpha1)

Service for retrieving immutable block data via signed cloud storage URLs.

**RPC Methods**: `FetchBlock`

### Database Service (v1alpha1)

Service for database lifecycle management with mutual exclusion enforcement.

**RPC Methods**: `CreateSnapshot`, `GetSnapshotStatus`, `ListSnapshots`, `DeleteSnapshot`, `VerifySnapshot`, `Restore`, `GetRestoreStatus`, `ListAvailableSnapshots`, `Truncate`, `GetTruncateStatus`, `StreamOperationProgress`, `GetOperationHistory`, `GetDatabaseInfo`, `CancelOperation`

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
