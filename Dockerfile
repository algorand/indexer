# This Dockerfile is used by Jenkins.
FROM ubuntu:18.04

ARG GO_VERSION=1.14
ENV DEBIAN_FRONTEND noninteractive
ENV USER root
ENV GOROOT=/usr/local/go
ENV GOPATH=$HOME/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH

RUN apt-get update && apt-get install -y apt-transport-https awscli ca-certificates build-essential curl git software-properties-common && \
    curl https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz | tar xzf - && \
    mv go /usr/local && \
    export PATH=/usr/local/go/bin:$PATH && \
    mkdir -p $HOME/go/src/github.com/algorand/indexer

RUN curl https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add - && \
    add-apt-repository "deb http://apt.postgresql.org/pub/repos/apt/ bionic-pgdg main" && \
    apt-get update && apt-get install -y postgresql-12 libpq-dev python3 python3-dev python3-psycopg2 python3-pip sudo && \
    pip3 install boto3 markdown2 "msgpack >=1" py-algorand-sdk

RUN go get github.com/vektra/mockery/.../

COPY ./ $HOME/go/src/github.com/algorand/indexer/
WORKDIR $HOME/go/src/github.com/algorand/indexer/
CMD ["/bin/bash"]

