ARG GO_IMAGE=golang:1.14.7
FROM $GO_IMAGE
ARG CHANNEL=stable

RUN echo "Go image: $GO_IMAGE"

# Misc dependencies
ENV HOME /opt
ENV DEBIAN_FRONTEND noninteractive

# BEGIN STUFF FOR just >--->
RUN wget -qO - 'https://proget.makedeb.org/debian-feeds/prebuilt-mpr.pub' | gpg --dearmor | tee /usr/share/keyrings/prebuilt-mpr-archive-keyring.gpg 1> /dev/null
RUN apt-get update && apt-get install -y lsb-release
RUN echo "deb [arch=amd64 signed-by=/usr/share/keyrings/prebuilt-mpr-archive-keyring.gpg] https://proget.makedeb.org prebuilt-mpr $(lsb_release -cs)" | tee /etc/apt/sources.list.d/prebuilt-mpr.list
RUN apt-get update && apt-get install -y just
# <---< STUFF FOR just ENDS HERE

RUN apt-get update && apt-get install -y apt-utils curl git git-core bsdmainutils python3 python3-pip make bash libtool libboost-math-dev libffi-dev wget

# Setup files for test
RUN mkdir -p /opt/go/indexer
COPY . /opt/go/indexer
WORKDIR /opt/go/indexer
RUN rm -f $HOME/go/bin/algod
RUN rm -f /opt/go/indexer/cmd/algorand-indexer/algorand-indexer
WORKDIR /opt/go/node
RUN wget https://raw.githubusercontent.com/algorand/go-algorand/rel/stable/cmd/updater/update.sh && chmod 744 update.sh
RUN ./update.sh -i -c $CHANNEL -n -d ./ -p /go/bin/
WORKDIR /opt/go/indexer
RUN make
ENV PATH="${HOME}/go/bin/:${PATH}"
WORKDIR /opt/go
RUN git clone --depth 1 -b bug-deltas-not-in-genesis https://github.com/algorand/conduit
WORKDIR /opt/go/conduit
RUN make
WORKDIR /opt/go/indexer/e2e_tests
RUN pip3 install ./
RUN mkdir -p /opt/tmp-conduit && cp /opt/go/indexer/e2e_tests/docker/indexer/conduit.yml /opt/tmp-conduit/

ENV INDEXER_DATA="${HOME}/indexer/"
WORKDIR /opt/go/indexer

RUN echo "Channel: $CHANNEL"

# Run test script
ENTRYPOINT ["/bin/bash", "-c", "env && sleep 5 && e2elive $EXTRA --connection-string \"$CONNECTION_STRING\" --s3-source-net \"$CI_E2E_FILENAME\" --indexer-bin /opt/go/indexer/cmd/algorand-indexer/algorand-indexer --indexer-port 9890 --conduit-bin /opt/go/conduit/cmd/conduit/conduit --conduit-dir /opt/tmp-conduit"]
