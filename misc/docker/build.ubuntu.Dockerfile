ARG ARCH
FROM indexer-builder:${ARCH}

ENV DEBIAN_FRONTEND="noninteractive"
ENV USER="root"

RUN apt update && \
    apt install -y software-properties-common python3-pip && \
    add-apt-repository ppa:deadsnakes/ppa -y && \
    apt install -y python3.9 python3.9-distutils && \
    update-alternatives --install /usr/bin/python python /usr/bin/python3.9 1 && \
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.9 1 && \
    python -m pip install markdown2 awscli
