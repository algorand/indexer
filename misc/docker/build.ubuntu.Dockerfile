ARG ARCH
FROM indexer-builder:${ARCH}

RUN apt update && \
    apt-get -y install python3-pip && \
    pip3 install markdown2
