# Bark Protocol Design Document

## Overview
The Bark protocol is a gRPC-based communication protocol for Dingo blockchain nodes. This repository contains the protocol buffer definitions and generated Go code for the Bark protocol.

Bark enables Dingo-to-Dingo communication for managing and operating Dingo instances. The protocol is designed with a modular, extensible architecture to support various operational aspects as they become needed.

The current protocol definitions cover the Archive Proxy and Database Service. Runtime server and operator implementations live in Dingo.

## Goals
- Dingo-to-Dingo Communication: Enable management and operation of Dingo instances
- Strong Contract: Use gRPC with Protocol Buffers to define clear and enforceable contracts between nodes
- Code Generation: Leverage buf's code generation for Go with ConnectRPC
- Extensibility: Design the protocol to support future operational needs
- Versioning: Start with v1alpha1 to allow for iterative development and breaking changes
- Buf Integration: Use buf to manage the protocol's lifecycle, including linting and code generation
- Modularity: Each protocol lives in its own module under the bark namespace

## Archive Proxy Protocol (v1alpha1)

The Archive Proxy enables transparent data proxying for immutable block data between two Dingo instances.

### Architecture

Server: Dingo instance running in archive mode
Client: Dingo instance configured to use the archive server for immutable data
Data Flow:
1. Client requests one or more blocks using BlockRef identifiers
2. Server returns signed URLs to CBOR files in cloud object storage
3. Client directly downloads blocks from cloud provider using signed URLs

### Protocol Definition

Location: `proto/v1alpha1/archive/archive.proto`

Service: ArchiveService
- FetchBlock(FetchBlockRequest) returns (FetchBlockResponse)

Messages:
- BlockRef: Identifies a block by hash, slot, and/or height
- FetchBlockRequest: Contains a list of BlockRef to fetch
- SignedUrl: Contains the block reference, signed URL, and expiration timestamp
- FetchBlockResponse: Returns signed URLs for found blocks and lists blocks not found

### Benefits

- Minimizes bandwidth and load on archive server (no data proxying)
- Leverages cloud provider infrastructure for data delivery
- Time-limited signed URLs provide security
- Suitable for immutable historical data

### Authentication

Optional mutual TLS support for client-server authentication


## Database Service Protocol (v1alpha1)

The Database Service provides a unified interface for database lifecycle management: snapshots, restoration, and chain truncation. Only one operation may run at a time; concurrent requests are rejected.

### Architecture

Server: Dingo instance exposing database management operations
Client: dingoctl or another Dingo instance acting as operator

### Protocol Definition

Location: `proto/v1alpha1/database/database.proto`

Service: DatabaseService

Snapshot operations:
- CreateSnapshot(CreateSnapshotRequest) returns (CreateSnapshotResponse)
- GetSnapshotStatus(GetSnapshotStatusRequest) returns (GetSnapshotStatusResponse)
- ListSnapshots(ListSnapshotsRequest) returns (ListSnapshotsResponse)
- DeleteSnapshot(DeleteSnapshotRequest) returns (DeleteSnapshotResponse)
- VerifySnapshot(VerifySnapshotRequest) returns (VerifySnapshotResponse)

Restore operations:
- Restore(RestoreRequest) returns (RestoreResponse)
- GetRestoreStatus(GetRestoreStatusRequest) returns (GetRestoreStatusResponse)
- ListAvailableSnapshots(ListAvailableSnapshotsRequest) returns (ListAvailableSnapshotsResponse)

Truncate operations:
- Truncate(TruncateRequest) returns (TruncateResponse)
- GetTruncateStatus(GetTruncateStatusRequest) returns (GetTruncateStatusResponse)

Shared operations:
- StreamOperationProgress(StreamOperationProgressRequest) returns (stream StreamOperationProgressResponse)
- GetOperationHistory(GetOperationHistoryRequest) returns (GetOperationHistoryResponse)
- GetDatabaseInfo(GetDatabaseInfoRequest) returns (GetDatabaseInfoResponse)
- CancelOperation(CancelOperationRequest) returns (CancelOperationResponse)

Key messages:
- BlockRef: Identifies a block by hash, slot, and/or block number
- OperationProgress: Tracks status, percent complete, and timestamps for an async operation
- OperationRecord: Summary entry in the history log
- SnapshotInfo: Metadata for a stored snapshot (id, name, tip, size, checksum, location)

### Truncate Semantics

The target block passed to Truncate is preserved and becomes the new chain tip. Only blocks with a strictly greater slot or block number are removed. The target may be identified by hash, slot, or block number; all supplied fields must be consistent.

### Mutual Exclusion

Only one of snapshot, restore, truncate, or verify may run at a time. A second request while an operation is in progress returns `FAILED_PRECONDITION`. Use `CancelOperation` to abort a running operation before starting another.

### Benefits

- Async operations with streamable progress (no client timeout risk on long-running jobs)
- Queryable history for operator auditing
- Remote object storage support for snapshots (S3, GCS, etc.)
- Clear cancellation and error semantics

## Versioning

Package naming: `bark.v1alpha1.{module}`
Current version: v1alpha1

Version stability:
- v1alpha1: Alpha phase - breaking changes are acceptable and expected
- v1beta1: (future) Beta phase - API stabilizing, limited breaking changes
- v1: (future) Stable - strong backward compatibility guarantees

## Technology Stack

Protocol: gRPC with Protocol Buffers
Code Generation: buf (https://buf.build)
Go Framework: ConnectRPC (connectrpc.com)
Build Tool: buf CLI
CI/CD: GitHub Actions

## Repository Structure

```
proto/v1alpha1/              - Protocol buffer definitions and generated Go code
  archive/                   - Archive Proxy protocol
    archive.proto            - Protocol buffer definition
    archive.pb.go            - Generated protobuf Go code
    archivev1alpha1connect/  - Generated ConnectRPC service code
  database/                  - Database Service protocol
    database.proto           - Protocol buffer definition
    database.pb.go           - Generated protobuf Go code
    databasev1alpha1connect/ - Generated ConnectRPC service code
buf.gen.yaml                 - Code generation configuration
buf.yaml                     - Buf module configuration
```

## Implementation Status

Completed:
- Protocol buffer definition for Archive Proxy (proto/v1alpha1/archive/archive.proto)
- Protocol buffer definition for Database Service (proto/v1alpha1/database/database.proto)
- Buf configuration and code generation setup
- Go module with ConnectRPC dependencies
- Generated Go code for v1alpha1 archive and database modules
- CI/CD pipelines for linting, testing, and dependency management
- Conventional commit enforcement

In Progress:
- Server implementation in Dingo (archive and database services)
- Client implementation in Dingo / dingoctl
- Integration testing

## Usage

To import the generated code in Go:

```go
import (
    archivev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/archive"
    "github.com/blinklabs-io/bark/proto/v1alpha1/archive/archivev1alpha1connect"

    databasev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/database"
    "github.com/blinklabs-io/bark/proto/v1alpha1/database/databasev1alpha1connect"
)
```

## References
- [Dingo Issue #335](https://github.com/blinklabs-io/dingo/issues/335) - Original requirements
- [Dingo Repository](https://github.com/blinklabs-io/dingo) - Blockchain node implementation
- [utxorpc/spec](https://github.com/utxorpc/spec) - Reference for repository organization
- [ConnectRPC](https://connectrpc.com) - RPC framework
- [Buf](https://buf.build) - Protocol buffer tooling

---

This document reflects the current state of the Bark protocol. Feedback and contributions are welcome!
