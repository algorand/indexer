ARG ARCH
FROM indexer-builder:${ARCH}

RUN apt update && \
    apt-get -y install \
    python3-pip \
    python3-setuptools \
    python3-wheel && \
    pip3 install markdown2
