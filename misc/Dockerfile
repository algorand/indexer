ARG GO_IMAGE=golang:1.14.7
FROM $GO_IMAGE

RUN echo "Go image: $GO_IMAGE"

# Misc dependencies
ENV HOME /opt
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y apt-utils curl git git-core bsdmainutils python3 python3-pip make bash libtool libboost-math-dev libffi-dev jq

# Setup files for test
RUN mkdir -p /opt/go/indexer
COPY . /opt/go/indexer
WORKDIR /opt/go/indexer
RUN rm /opt/go/indexer/cmd/algorand-indexer/algorand-indexer
RUN make
RUN pip3 install -r misc/requirements.txt
ENV GOALGORAND="/opt/go/indexer/third_party/go-algorand"
# Install submodule go-algorand binaries to the path
RUN cd $GOALGORAND && make install
ENV PATH="${GOPATH}/bin:${PATH}"
# buildtestdata.sh requires /usr/bin/env python to work
RUN ln -s $(which python3) /usr/local/bin/python
# Generate test data from the submodule
RUN /opt/go/indexer/misc/buildtestdata.sh
RUN find "${HOME}/Algorand/e2edata/net_done.tar.bz2"

# Run test script
ENTRYPOINT ["/bin/bash", "-c", "sleep 5 && python3 misc/e2elive.py --connection-string \"$CONNECTION_STRING\" --indexer-bin /opt/go/indexer/cmd/algorand-indexer/algorand-indexer --indexer-port 9890 --source-net \"$HOME/Algorand/e2edata/net_done.tar.bz2\""]
