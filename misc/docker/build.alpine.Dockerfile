ARG ARCH
FROM indexer-builder:${ARCH}

RUN apk update && \
    apk add --update \
    py3-pip \
    pip3 install markdown2 awscli
