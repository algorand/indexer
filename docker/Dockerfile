FROM golang:alpine

# Dependencies
RUN apk add --update make

# Install test data
COPY docker/testdata.tar.bz2 /tmp/testdata.tar.bz2
RUN tar -jxf /tmp/testdata.tar.bz2 -C /tmp

# Add code to gopath and build
RUN mkdir -p src/github.com/algorand/indexer
WORKDIR src/github.com/algorand/indexer
COPY . .
RUN make

# Launch indexer with a script
COPY docker/run.sh /tmp/run.sh
CMD ["/tmp/run.sh"]
