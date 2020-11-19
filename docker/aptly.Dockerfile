FROM ubuntu:18.04

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && apt-get install aptly awscli build-essential curl gnupg2 -y

WORKDIR /root
COPY .aptly.conf .
RUN curl https://releases.algorand.com/key.pub | gpg --no-default-keyring --keyring trustedkeys.gpg --import - && \
    aptly mirror create indexer https://releases.algorand.com/deb-test/ indexer main && \
    aptly repo create -distribution=indexer -architectures=amd64 -component=main -comment=indexer indexer && \
    aptly mirror update indexer && \
    aptly repo import indexer indexer algorand-indexer
CMD ["/bin/bash"]

