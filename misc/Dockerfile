ARG GO_IMAGE=golang:1.14.7
FROM $GO_IMAGE

RUN echo "Go image: $GO_IMAGE"

# Misc dependencies
ENV HOME /opt
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y apt-utils curl git git-core bsdmainutils python3 python3-pip make bash libtool libboost-math-dev libffi-dev

# Setup files for test
RUN mkdir -p /opt/go/indexer
COPY . /opt/go/indexer
WORKDIR /opt/go/indexer
RUN rm -f $HOME/go/bin/algod
RUN rm /opt/go/indexer/cmd/algorand-indexer/algorand-indexer
WORKDIR /opt/go/indexer/third_party/go-algorand
RUN make install
WORKDIR /opt/go/indexer
RUN make
ENV PATH="${HOME}/go/bin/:${PATH}"
RUN pip3 install -r misc/requirements.txt

ENV INDEXER_DATA="${HOME}/indexer/"
# Run test script
ENTRYPOINT ["/bin/bash", "-c", "sleep 5 && python3 misc/e2elive.py --connection-string \"$CONNECTION_STRING\" --s3-source-net \"$CI_E2E_FILENAME\" --indexer-bin /opt/go/indexer/cmd/algorand-indexer/algorand-indexer --indexer-port 9890"]
