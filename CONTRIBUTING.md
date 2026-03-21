# Contributing

## Source Code Changes

**Accepted:** Bug fixes, security fixes, simplifications, reducing code.

**Not accepted:** Features, capabilities, compatibility, enhancements. These should be built into the core Go packages or added as skills if they involve external integrations.

## Architecture

NanoClaw is written in Go 1.26+.

- `cmd/nanoclaw/main.go`: Entry point and orchestration
- `pkg/`: Core logic and channel implementations
- `container/`: Agent runner (Node.js) that runs inside isolated containers

## Development

1. Install Go 1.26+
2. Clone the repository
3. Run `make build` to build the orchestrator and setup tools
4. Run `make test` to run all tests

## Testing

Always add tests for new logic in the corresponding `*_test.go` files. We aim for high coverage of core orchestration and routing logic.
