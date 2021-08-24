ARG ARCH
FROM indexer-builder:${ARCH}

ENV USER="root"

RUN apk update && \
    apk add --update \
    py3-pip \
    py3-setuptools \
    py3-wheel && \
    pip3 install markdown2 awscli
