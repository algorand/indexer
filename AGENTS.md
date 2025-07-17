# Agent Development Guide

This file contains essential development commands for the Algorand Indexer project.

## Build Commands

- `make` or `make all` - Build everything (algorand-indexer binary, postgres schema, mocks)
- `make cmd/algorand-indexer/algorand-indexer` - Build the main indexer binary
- `make check` - Check that all packages compile

## Testing Commands

- `make test` - Run tests with coverage
- `make e2e` - Run end-to-end tests using Docker
- `make e2e-filter-test` - Run filtered e2e tests
- `make indexer-v-algod` - Run parity tests between indexer and algod
- `make test-generate` - Run test generation script
- `make test-package` - Run package tests

## Code Quality Commands

- `make lint` - Run linting (golangci-lint and go vet)
- `make fmt` - Format Go code

## API Generation Commands

- `cd api && make` or `cd api && make all` - Generate API code from OpenAPI spec
- `cd api && make generate` - Generate API code (replaces old generate.sh)
- `cd api && make clean` - Clean generated API files
- `cd api && make oapi-codegen` - Install oapi-codegen tool

## Development Dependencies

- `make idb/mocks/IndexerDb.go` - Generate mocks for IndexerDb interface
- `make idb/postgres/internal/schema/setup_postgres_sql.go` - Generate postgres schema

## Deployment Commands

- `make deploy` - Deploy using mule/deploy.sh
- `make sign` - Sign packages using mule/sign.sh

## Common Development Workflow

1. Make code changes
2. Run `make fmt` to format code
3. Run `make lint` to check for issues
4. Run `make test` to run tests
5. Run `make check` to ensure everything compiles
6. For API changes: `cd api && make` to regenerate API code