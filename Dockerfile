FROM ubuntu:18.04

ARG GO_VERSION=1.13.11
ENV DEBIAN_FRONTEND noninteractive
ENV USER root
ENV GOROOT=/usr/local/go
ENV GOPATH=$HOME/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH

RUN apt-get update && apt-get -y install apt-transport-https ca-certificates software-properties-common build-essential curl git python3 python3-pip && \
    curl https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz | tar xzf - && \
    mv go /usr/local && \
    mkdir -p $HOME/go/src/github.com/algorand/indexer && \
    pip3 install markdown2

COPY ./ $HOME/go/src/github.com/algorand/indexer/
WORKDIR $HOME/go/src/github.com/algorand/indexer/
CMD ["/bin/bash"]

