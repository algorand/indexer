ARG GO_IMAGE=golang:1.14.7
FROM $GO_IMAGE
ARG CHANNEL=stable
ARG CI_E2E_FILENAME

RUN echo "Go image: $GO_IMAGE"

# Misc dependencies
ENV HOME /opt
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y apt-utils curl git git-core bsdmainutils python3 python3-pip make bash libtool libboost-math-dev libffi-dev wget gettext-base

# Setup files for test
RUN mkdir -p /opt/go/indexer
RUN mkdir -p /opt/conduit-dir
RUN mkdir -p /opt/algod-dir
COPY . /opt/go/indexer
WORKDIR /opt/go/indexer
RUN rm -f $HOME/go/bin/algod
RUN rm -f /opt/go/indexer/cmd/conduit/conduit
WORKDIR /opt/go/node
RUN wget https://raw.githubusercontent.com/algorand/go-algorand/rel/stable/cmd/updater/update.sh && chmod 744 update.sh
RUN ./update.sh -i -c $CHANNEL -n -d ./ -p /go/bin/
WORKDIR /opt/go/indexer
RUN make
ENV PATH="${HOME}/go/bin/:${PATH}"
WORKDIR ./e2e_tests
RUN pip3 install ./

ENV INDEXER_DATA="${HOME}/indexer/"
WORKDIR /opt/go/indexer

# Retrieve the network data
RUN get-test-data --s3-source-net ${CI_E2E_FILENAME} --algod-dir /opt/algod-dir

# Run test script
CMD ["/bin/bash", "-c", \
"goal network start -r /opt/algod-dir/net/ && \
export ALGOD_ADDR=http://$(cat /opt/algod-dir/net/Node/algod.net) && \
export ALGOD_TOKEN=$(cat /opt/algod-dir/net/Node/algod.token) && \
cat ./e2e_tests/docker/indexer-filtered/e2e-write/conduit.yml | envsubst > /opt/conduit-dir/conduit.yml && \
/opt/go/indexer/cmd/conduit/conduit -d /opt/conduit-dir"]