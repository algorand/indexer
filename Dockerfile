# Build this Dockerfile with goreleaser.
# The binary must be present at /algorand-indexer
FROM debian:bullseye-slim

# Hard code UID/GID to 999 for consistency in advanced deployments.
# Install ca-certificates to enable using infra providers.
# Install gosu for fancy data directory management.
RUN groupadd --gid=999 --system algorand && \
    useradd --uid=999 --no-log-init --create-home --system --gid algorand algorand && \
    mkdir -p /data && \
    chown -R algorand.algorand /data && \
    apt-get update && \
    apt-get install -y --no-install-recommends gosu ca-certificates && \
    update-ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY algorand-indexer /usr/local/bin/algorand-indexer
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENV INDEXER_DATA /data
WORKDIR /data
# Note: docker-entrypoint.sh calls 'algorand-indexer'. Similar entrypoint scripts
# accept the binary as the first argument in order to surface a suite of
# tools (i.e. algod, goal, algocfg, ...). Maybe this will change in the
# future, but for now this approach seemed simpler.
ENTRYPOINT ["docker-entrypoint.sh"]
