#!/usr/bin/env bash
set -e

rootdir=`dirname $0`
pushd $rootdir

# Convert v2 to v3
curl -s -X POST "https://converter.swagger.io/api/convert" -H "accept: application/json" -H "Content-Type: application/json" -d @./indexer.oas2.json  -o 3.json

python3 -c "import json; import sys; json.dump(json.load(sys.stdin), sys.stdout, indent=2, sort_keys=True)" < 3.json > indexer.oas3.yml
rm 3.json

echo "generating code."
oapi-codegen -package generated -type-mappings integer=uint64 -generate types -o generated/v2/types.go -exclude-tags=common indexer.oas3.yml
oapi-codegen -package generated -type-mappings integer=uint64 -generate server,spec -o generated/v2/routes.go -exclude-tags=common indexer.oas3.yml

oapi-codegen -package common -type-mappings integer=uint64 -generate types -o generated/common/types.go -include-tags=common indexer.oas3.yml
oapi-codegen -package common -type-mappings integer=uint64 -generate server,spec -o generated/common/routes.go -include-tags=common indexer.oas3.yml
