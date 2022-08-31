#!/usr/bin/env bash
cd third_party/go-algorand && git fetch && git checkout rel/nightly
commit_hash=$(git log --format='%H %an' -10 | grep -v ci-bot | cut -d ' ' -f1 | head -n 1 | cut -c1-8)
echo "using commit: ${commit_hash}"
git checkout master && git checkout $commit_hash
