# Bark Protocol Design Document

## Overview
The Bark protocol is a gRPC-based communication protocol for Dingo blockchain nodes. This repository contains the protocol buffer definitions and generated Go code for the Bark protocol.

Bark enables Dingo-to-Dingo communication for managing and operating Dingo instances. The protocol is designed with a modular, extensible architecture to support various operational aspects as they become needed.

The current protocol definitions cover the Archive Proxy, Database Service, and Lifecycle Service. Runtime server and operator implementations live in Dingo.

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

### Authorization

Per [dingo#2988](https://github.com/blinklabs-io/dingo/issues/2988), DatabaseService exposes operations that alter persistent state or interrupt availability and must not be reachable by anonymous callers. Servers require mutual TLS (mTLS) and must verify the client certificate before dispatching any RPC on this service, per the same transport-layer model as LifecycleService.

RPCs are split by sensitivity:
- Read-only: `ListSnapshots`, `ListAvailableSnapshots`, `GetSnapshotStatus`, `GetRestoreStatus`, `GetTruncateStatus`, `StreamOperationProgress`, `GetOperationHistory`, `GetDatabaseInfo`
- Destructive/mutating (require an authenticated operator identity): `CreateSnapshot`, `DeleteSnapshot`, `VerifySnapshot`, `Restore`, `Truncate`, `CancelOperation`

### Benefits

- Async operations with streamable progress (no client timeout risk on long-running jobs)
- Queryable history for operator auditing
- Remote object storage support for snapshots (S3, GCS, etc.)
- Clear cancellation and error semantics

## Lifecycle Service Protocol (v1alpha1)

The Lifecycle Service provides remote process-lifecycle control for a Dingo node: graceful stop, graceful restart, and status reporting. It exists to enable fleet management, maintenance, and rolling updates without requiring direct OS-level access (e.g. SSH + systemctl) to the host.

### Architecture

Server: Dingo instance exposing lifecycle control
Client: dingoctl or another Dingo instance acting as operator

### Protocol Definition

Location: `proto/v1alpha1/lifecycle/lifecycle.proto`

Service: LifecycleService

- Stop(StopRequest) returns (StopResponse)
- Restart(RestartRequest) returns (RestartResponse)
- GetStatus(GetStatusRequest) returns (GetStatusResponse)

Key messages:
- StopRequest / RestartRequest: carry an optional `graceful_timeout`; unset or zero means the server's configured default is used
- StopResponse / RestartResponse: report the `effective_timeout` that will actually be enforced and the `deadline` by which the graceful sequence must finish
- GetStatusResponse: reports `LifecycleState`, `HealthStatus`, process `uptime`, `version`, and `SyncStatus` (synced flag, current tip, estimated network tip slot, slots behind)

### Shutdown Sequence

Stop and Restart both trigger the same shutdown coordinator sequence: cease chain sync, drain in-flight connections, flush the database, then exit (Stop) or re-execute the same binary and arguments in place (Restart). If the sequence does not finish within the effective timeout, the coordinator forces the process to exit (or re-execute) rather than hang indefinitely. While draining, `GetStatus` reports `LIFECYCLE_STATE_STOPPING` or `LIFECYCLE_STATE_RESTARTING` along with the enforced deadline, so callers can poll for the transition instead of needing a dedicated streaming RPC.

### Mutual Exclusion

Only one of Stop or Restart may be in progress at a time. A second request received while one is already underway is rejected with `FAILED_PRECONDITION`.

### Internal Event Emission

Per [dingo#1653](https://github.com/blinklabs-io/dingo/issues/1653), Dingo emits lifecycle events (stopping, restarting, etc.) on its existing internal event bus as the shutdown coordinator runs, so other in-process subsystems can react to a pending stop/restart. This is purely internal to Dingo and is not part of the Bark wire contract — no LifecycleService RPC carries it. The externally-observable equivalent for remote callers is `GetStatus`'s `LifecycleState` transitioning to `LIFECYCLE_STATE_STOPPING`/`LIFECYCLE_STATE_RESTARTING`; a dedicated streaming RPC was deliberately not added for this since polling `GetStatus` covers the network-facing need.

### Authorization

Stop and Restart can take a node offline, so servers MUST reject anonymous or unauthenticated callers for every RPC on this service. Per [dingo#1653](https://github.com/blinklabs-io/dingo/issues/1653), authentication/authorization is enforced at the transport layer rather than within the service itself: servers require mutual TLS (mTLS) and must verify the client certificate presented on the connection before dispatching any LifecycleService RPC. This mirrors the mTLS support already defined for the Archive Proxy protocol, except here it is mandatory rather than optional — a plain-text or server-TLS-only connection must be refused for this service. Dingo is responsible for enforcing this at the server; Bark defines the contract and expected behavior.

### Benefits

- Enables fleet management and rolling updates without SSH access to the host
- Graceful timeout is explicit and observable via `GetStatus`, with a documented forced-exit fallback
- Restart re-executes in place, picking up a replaced binary or changed configuration without external process supervision

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
  lifecycle/                 - Lifecycle Service protocol
    lifecycle.proto          - Protocol buffer definition
    lifecycle.pb.go          - Generated protobuf Go code
    lifecyclev1alpha1connect/ - Generated ConnectRPC service code
buf.gen.yaml                 - Code generation configuration
buf.yaml                     - Buf module configuration
```

## Implementation Status

Completed:
- Protocol buffer definition for Archive Proxy (proto/v1alpha1/archive/archive.proto)
- Protocol buffer definition for Database Service (proto/v1alpha1/database/database.proto)
- Protocol buffer definition for Lifecycle Service (proto/v1alpha1/lifecycle/lifecycle.proto)
- Buf configuration and code generation setup
- Go module with ConnectRPC dependencies
- Generated Go code for v1alpha1 archive, database, and lifecycle modules
- CI/CD pipelines for linting, testing, and dependency management
- Conventional commit enforcement

In Progress:
- Server implementation in Dingo (archive, database, and lifecycle services)
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

    lifecyclev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/lifecycle"
    "github.com/blinklabs-io/bark/proto/v1alpha1/lifecycle/lifecyclev1alpha1connect"
)
```

## References
- [Dingo Issue #335](https://github.com/blinklabs-io/dingo/issues/335) - Original requirements
- [Dingo Issue #1653](https://github.com/blinklabs-io/dingo/issues/1653) - Remote lifecycle service requirements
- [Dingo Repository](https://github.com/blinklabs-io/dingo) - Blockchain node implementation
- [utxorpc/spec](https://github.com/utxorpc/spec) - Reference for repository organization
- [ConnectRPC](https://connectrpc.com) - RPC framework
- [Buf](https://buf.build) - Protocol buffer tooling

---

This document reflects the current state of the Bark protocol. Feedback and contributions are welcome!
