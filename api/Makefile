# `make all` or just `make` should be appropriate for dev work
all:	generated/v2/types.go generated/v2/routes.go generated/common/types.go generated/common/routes.go

# `make generate` should be able to replace old `generate.sh` script and be appropriate for build system use
generate:	oapi-codegen all

generated/v2/types.go:	indexer.oas3.yml
	oapi-codegen -package generated -type-mappings integer=uint64 -generate types -exclude-tags=common -o ./generated/v2/types.go indexer.oas3.yml

generated/v2/routes.go:	indexer.oas3.yml
	oapi-codegen -package generated -type-mappings integer=uint64 -generate server,spec -exclude-tags=common -o ./generated/v2/routes.go indexer.oas3.yml

generated/common/types.go:	indexer.oas3.yml
	oapi-codegen -package common -type-mappings integer=uint64 -generate types -include-tags=common -o ./generated/common/types.go indexer.oas3.yml

generated/common/routes.go:	indexer.oas3.yml
	oapi-codegen -package common -type-mappings integer=uint64 -generate server,spec -include-tags=common -o ./generated/common/routes.go indexer.oas3.yml

indexer.oas3.yml:	.3tmp.json
	python3 jsoncanon.py < .3tmp.json > indexer.oas3.yml

.3tmp.json:	indexer.oas2.json
	curl -s -X POST "https://converter.swagger.io/api/convert" -H "accept: application/json" -H "Content-Type: application/json" -d @./indexer.oas2.json -o .3tmp.json

oapi-codegen:	.PHONY
	GO111MODULE=on go get -u "github.com/algorand/oapi-codegen/...@v1.3.5-algorand5"

.PHONY:
