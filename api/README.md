# API Design

We are using a documentation driven process.

The API is defined using [OpenAPI v2](https://swagger.io/specification/v2/) in **indexer.oas2.yml**.

## Updating REST API

The Makefile will install our fork of **oapi-codegen**, use `make oapi-codegen` to install it directly.

1. Document your changes by editing **indexer.oas2.json**
2. Regenerate the endpoints by running **generate.sh**. The sources at **generated/** will be updated.
3. Update the implementation in **handlers.go**. It is sometimes useful to consult **generated/routes.go** to make sure the handler properly implements **ServerInterface**.

## What codegen tool is used?

We found that [oapi-codegen](https://github.com/deepmap/oapi-codegen) produced the cleanest code, and had an easy to work with codebase. There is an algorand fork of this project which contains a couple modifications that were needed to properly support our needs.

Specifically, `uint64` types aren't strictly supported by OpenAPI. So we added a type-mapping feature to oapi-codegen.

## Why do we have indexer.oas2.yml and indexer.oas3.yml?

We chose to maintain V2 and V3 versions of the spec because OpenAPI v3 doesn't seem to be widely supported. Some tools worked better with V3 and others with V2, so having both available has been useful. To reduce developer burdon, the v2 specfile is automatically converted v3 using [converter.swagger.io](http://converter.swagger.io/).
