#!/bin/bash

usage() { echo "Usage: $0 <-a ARCH> [-p] [-t]" 1>&2; exit "$1"; }

# This does not work with docker buildkit
export DOCKER_BUILDKIT=0

SCRIPT_PATH=$(dirname "${0}")
MAKE_TARGET='fakepackage'
ARCH='amd64'
DOCKER_RUN_OPTS='-i'

while getopts "a:pth" o; do
    case "${o}" in
        a)
            ARCH="${OPTARG}"
            ;;
        p)
            MAKE_TARGET='package'
            ;;
        t)
            DOCKER_RUN_OPTS='-ti'
            ;;
        *)
            usage 2
            ;;
    esac
done

if [[ ! "$ARCH" =~ ^(amd64|arm|arm64)$ ]]; then
    echo 'ARCH must be either amd64, arm or arm64'
    usage 2
fi

export GOALGORAND_DOCKERFILE_PATH="${SCRIPT_PATH}/../third_party/go-algorand/docker/build/cicd.ubuntu.Dockerfile"
export INDEXER_DOCKERFILE_PATH="${SCRIPT_PATH}/../misc/docker/build.ubuntu.Dockerfile"
export DOCKER_ARCH='amd64'

if [ "${ARCH}" == 'arm' ]; then
    GOALGORAND_DOCKERFILE_PATH="${SCRIPT_PATH}/../third_party/go-algorand/docker/build/cicd.alpine.Dockerfile"
    INDEXER_DOCKERFILE_PATH="${SCRIPT_PATH}/../misc/docker/build.alpine.Dockerfile"
    DOCKER_ARCH='arm32v6'
elif [ "${ARCH}" == 'arm64' ]; then
    DOCKER_ARCH='arm64v8'
fi

git submodule update --init
export BUILD_IMAGE=indexer-builder:${DOCKER_ARCH}
export GOLANG_VERSION=$(${SCRIPT_PATH}/../third_party/go-algorand/scripts/get_golang_version.sh)
export VERSION=$(cat ${SCRIPT_PATH}/../.version)

docker build -t "${BUILD_IMAGE}" \
    -f ${GOALGORAND_DOCKERFILE_PATH} \
    --build-arg ARCH="${DOCKER_ARCH}" \
    --build-arg GOLANG_VERSION=${GOLANG_VERSION} \
    third_party/go-algorand
docker build -t "${BUILD_IMAGE}" \
    --build-arg ARCH="${DOCKER_ARCH}" \
    - < ${INDEXER_DOCKERFILE_PATH}
docker run ${DOCKER_RUN_OPTS} \
    -v `pwd`:/go/src/github.com/algorand/indexer \
    --workdir /go/src/github.com/algorand/indexer \
    "${BUILD_IMAGE}" \
    bash -c "make ${MAKE_TARGET}"
