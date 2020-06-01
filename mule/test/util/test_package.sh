#!/usr/bin/env bash

set -ex

trap cleanup 0

GO_VERSION=1.13.11
OS_LIST=(
    ubuntu:18.04
    ubuntu:20.04
)

FAILED=()

build_images () {
    # We'll use this simple tokenized Dockerfile.
    # https://serverfault.com/a/72511
    TOKENIZED=$(echo -e "\
FROM {{OS}}\n\n\
RUN apt-get update && apt-get install curl -y && \
    curl https://dl.google.com/go/go$GO_VERSION.linux-amd64.tar.gz | tar xzf - && \
    mv go /usr/local
WORKDIR /root\n\
COPY . .\n\
CMD [\"/bin/bash\"]")

    for item in ${OS_LIST[*]}
    do
        # Use pattern substitution here (like sed).
        # ${parameter/pattern/substitution}
        echo -e "${TOKENIZED/\{\{OS\}\}/$item}" > Dockerfile
        if ! docker build -t "${item}-run-tests" .
        then
            FAILED+=("$item")
        fi
    done
}

run_images () {
    for item in ${OS_LIST[*]}
    do
        echo "[$0] Running ${item}-test..."

        if ! docker run --rm --name algorand -e OS_TYPE="$OS_TYPE" -e ARCH="$ARCH" -e WORKDIR="$WORKDIR" --volumes-from "$HOSTNAME" -t "${item}-run-tests" bash ./mule/test/tests/run_tests -b "$BRANCH" -c "$CHANNEL" -h "$SHA" -r "$VERSION"
        then
            FAILED+=("$item")
        fi
    done
}

cleanup() {
    rm -f Dockerfile
}

check_failures() {
    if [ "${#FAILED[@]}" -gt 0 ]
    then
        echo -e "\n[$0] The following images could not be $1:"

        for failed in ${FAILED[*]}
        do
            echo " - $failed"
        done

        echo
        exit 1
    fi
}

build_images
check_failures built
echo "[$0] All builds completed with no failures."

run_images
check_failures verified
echo "[$0] All runs completed with no failures."

