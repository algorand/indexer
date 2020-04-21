#!/usr/bin/env bash
set -e

rootdir=`dirname $0`
pushd $rootdir

# Convert v2 to v3
curl -s -X POST "https://converter.swagger.io/api/convert" -H "accept: application/json" -H "Content-Type: application/json" -d @./indexer.oas2.json  -o 3.json
cat 3.json | json_pp > indexer.oas3.yml
rm 3.json
sed -i 's/\*.\*/application\/json/g' indexer.oas3.yml

echo "generating code."
oapi-codegen -package generated -type-mappings integer=uint64 -generate types -o generated/types.go indexer.oas3.yml
oapi-codegen -package generated -type-mappings integer=uint64 -generate server,spec -o generated/routes.go indexer.oas3.yml

