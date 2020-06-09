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

# Install postgres and python3 for packaging and e23 tests.
# Note: may need psycopg2-binary package.
RUN curl https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add - && \
    add-apt-repository "deb http://apt.postgresql.org/pub/repos/apt/ bionic-pgdg main" && \
    apt-get update && apt-get install -y postgresql-12 python3 python3-pip && \
    pip3 install boto3 markdown2 "msgpack >=1" psycopg2 py-algorand-sdk && \
    systemctl start postgresql.service && \
    sudo -u postgres bash -c "psql -c \"CREATE ROLE $USER WITH SUPERUSER CREATEDB LOGIN ENCRYPTED PASSWORD '$USER';\""

RUN go get github.com/vektra/mockery/.../

#RUN add-apt-repository "deb https://releases.algorand.com/deb/ stable main" && \
#    curl https://releases.algorand.com/key.pub | apt-key add - && \
#    apt-get update && \
#    apt-get install algorand-indexer -y

COPY ./ $HOME/go/src/github.com/algorand/indexer/
WORKDIR $HOME/go/src/github.com/algorand/indexer/
CMD ["/bin/bash"]

