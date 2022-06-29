ARG ARCH="amd64"
FROM ${ARCH}/ubuntu:20.04
ARG GOLANG_VERSION
ARG GOARCH="amd64"
ARG ARCH="amd64"
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y \
        build-essential \
        git \
        libboost-all-dev \
        wget \
        sqlite3 \
        autoconf \
        jq \
        bsdmainutils \
        shellcheck \
        awscli \
        python3-pip && \
    pip3 install markdown2
WORKDIR /root
RUN wget https://dl.google.com/go/go${GOLANG_VERSION}.linux-${GOARCH}.tar.gz \
    && tar -xvf go${GOLANG_VERSION}.linux-${GOARCH}.tar.gz && \
    mv go /usr/local
ENV GOROOT=/usr/local/go \
    GOPATH=$HOME/go \
    ARCH_TYPE=${ARCH}
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH \
    GOPROXY=https://proxy.golang.org,https://pkg.go.dev,https://goproxy.io,direct
WORKDIR $GOPATH/src/github.com/algorand/go-algorand
RUN echo "vm.max_map_count = 262144" >> /etc/sysctl.conf
CMD ["/bin/bash"]
