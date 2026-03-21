# Go Port Implementation Plan

This document tracks the phased implementation of porting NanoClaw to Go 1.26+.

## Phase 1: Foundation (Zero Dependencies)
- [x] 1. **`pkg/env`**: Implement `ReadEnvFile` and test it (matches `src/env.ts`).
- [x] 2. **`pkg/config`**: Implement config loading and constants, with tests (matches `src/config.ts`).
- [x] 3. **`pkg/router` (Formatting)**: Implement `formatMessages` and `formatOutbound`, with tests (matches formatting in `src/router.ts`).
- [x] 4. **`pkg/mount`**: Implement mount allowlist validation, with tests (matches `src/mount-security.ts`).
- [x] 5. **`pkg/senderallowlist`**: Implement sender allowlist validation, with tests (matches `src/sender-allowlist.ts`).

## Phase 2: Database Layer
- [x] 6. **`pkg/db` (Schema)**: Implement SQLite database initialization and schema migration (matches `src/db.ts`).
- [x] 7. **`pkg/db` (Queries)**: Implement Storage interface functions for messages, groups, sessions, tasks, router_state, and chat metadata, with tests.

## Phase 3: Core Execution Primitives
- [x] 8. **`pkg/container` (Mounts & Args)**: Implement volume mount building and container arg building, with tests.
- [x] 9. **`pkg/container` (Runner)**: Implement `RunContainerAgent` with streaming, timeouts, and logs, with tests (matches `src/container-runner.ts`).
- [x] 10. **`pkg/proxy`**: Port the credential proxy from Express to Go (matches `src/credential-proxy.ts`).

## Phase 4: IPC and Channels Foundation
- [x] 11. **`pkg/channel`**: Implement channel interface, registry, and factory system (matches `src/channels/registry.ts`).
- [x] 12. **`pkg/router` (Routing)**: Implement `findChannel` functionality, with tests.
- [x] 13. **`pkg/ipc`**: Implement the file watcher, IPC event handling, and callbacks, with tests (matches `src/ipc.ts`).

## Phase 5: Messaging Loop & Scheduler
- [x] 14. **`pkg/taskqueue`**: Implement `GroupQueue` for message pipelining and concurrency control, with tests (matches `src/group-queue.ts`).
- [x] 15. **`pkg/messagepoller`**: Implement the polling loop for new messages.
- [x] 16. **`pkg/scheduler`**: Implement the task scheduler loop, with tests (matches `src/task-scheduler.ts`).

## Phase 6: Channel Implementations
- [x] 17. **`pkg/channels/telegram`**: Implement Telegram channel in Go (matches `src/channels/telegram.ts`).

## Phase 7: Entry Point & Integration
- [x] 18. **`cmd/nanoclaw`**: Tie everything together in `main.go` (start DB, proxy, IPC, queue, message loop, channels).

## Phase 8: Tooling & Deployment
- [x] 19. **`Makefile`**: Create a comprehensive Makefile for building, testing, and running.
- [x] 20. **`Dockerfile`**: Update root Dockerfile for multi-stage Go builds.
- [x] 21. **`launchd/com.nanoclaw.plist`**: Update service definition for the Go binary.

## Phase 9: Additional Channels
- [x] 22. **`pkg/channels/slack`**: Implement Slack channel in Go (matches `src/channels/slack.ts`).
- [x] 23. **`pkg/channels/discord`**: Implement Discord channel in Go (matches `src/channels/discord.ts`).
- [x] 24. **`pkg/channels/gmail`**: Implement Gmail channel in Go (matches `src/channels/gmail.ts`).
- [x] 25. **`pkg/channels/whatsapp`**: Implement WhatsApp channel in Go (using `whatsmeow`).

## Phase 10: Setup Tooling Port
- [x] 26. **`pkg/setup`**: Port remaining setup steps (Environment, Container, Groups, Register, Mounts, Service, Verify).
- [x] 27. **`cmd/setup`**: Create a Go setup entry point.

## Phase 11: Feature Parity & Robustness
- [x] 28. **`pkg/channel`**: Standardize `SetTyping` and `SyncGroups` interfaces.
- [x] 29. **`pkg/channels/gmail`**: Add unit tests.
- [x] 30. **`pkg/channels/whatsapp`**: Add unit tests and handle QR code more robustly for headless.
- [x] 31. **`pkg/channels`**: Implement `SyncGroups` for supported channels.

## Phase 12: Final Verification
- [x] 32. **Full End-to-End Test**: Verify with a real container agent.
- [x] 33. **Documentation**: Update README.md and CONTRIBUTING.md for Go.
