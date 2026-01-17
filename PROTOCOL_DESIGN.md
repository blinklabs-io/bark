# Bark Protocol Design Document

## Overview
The Bark protocol is a gRPC-based communication protocol for Dingo blockchain nodes. This repository contains the protocol buffer definitions and generated Go code for the Bark protocol.

Bark enables Dingo-to-Dingo communication for managing and operating Dingo instances. The protocol is designed with a modular, extensible architecture to support various operational aspects as they become needed.

The current implementation is the Archive Proxy protocol, which enables efficient retrieval of immutable blockchain data via signed URLs to cloud object storage.

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
proto/v1alpha1/            - Protocol buffer definitions and generated Go code
  archive/                 - Archive Proxy protocol
    archive.proto          - Protocol buffer definition
    archive.pb.go          - Generated protobuf Go code
    archivev1alpha1connect/ - Generated ConnectRPC service code
buf.gen.yaml               - Code generation configuration
buf.yaml                   - Buf module configuration
```

## Implementation Status

Completed:
- Protocol buffer definition for Archive Proxy (proto/v1alpha1/archive/archive.proto)
- Buf configuration and code generation setup
- Go module with ConnectRPC dependencies
- Generated Go code for v1alpha1 archive module
- CI/CD pipelines for linting, testing, and dependency management
- Conventional commit enforcement

In Progress:
- Server implementation in Dingo
- Client implementation in Dingo
- Integration testing

## Usage

To import the generated code in Go:

```go
import (
    archivev1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/archive"
    "github.com/blinklabs-io/bark/proto/v1alpha1/archive/archivev1alpha1connect"
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
