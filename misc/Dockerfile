ARG GO_IMAGE=golang:1.14.7
FROM $GO_IMAGE

RUN echo "Go image: $GO_IMAGE"

# Misc dependencies
ENV HOME /opt
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y apt-utils curl git git-core bsdmainutils python3 python3-pip make bash libtool libboost-math-dev libffi-dev

# Install algod nightly binaries to the path
RUN mkdir -p /opt/algorand/{bin,data}
ADD https://github.com/algorand/go-algorand/raw/1e1474216421da27008726c44ebe0a5ba2fb6a08/cmd/updater/update.sh /opt/algorand/bin/update.sh
RUN chmod 755 /opt/algorand/bin/update.sh
WORKDIR /opt/algorand/bin
RUN ./update.sh -i -c nightly -p /opt/algorand/bin -d /opt/algorand/data -n
RUN find /opt/algorand
ENV PATH="/opt/algorand/bin:${PATH}"

# Setup files for test
RUN mkdir -p /opt/go/indexer
COPY . /opt/go/indexer
WORKDIR /opt/go/indexer
RUN rm /opt/go/indexer/cmd/algorand-indexer/algorand-indexer
RUN make
RUN pip3 install -r misc/requirements.txt

# Run test script
ENTRYPOINT ["/bin/bash", "-c", "sleep 5 && python3 misc/e2elive.py --connection-string \"$CONNECTION_STRING\" --indexer-bin /opt/go/indexer/cmd/algorand-indexer/algorand-indexer --indexer-port 9890"]
