#!/usr/bin/env bash
set -e

# To allow mounting the data directory we need to change permissions
# to our algorand user. The script is initially run as the root user
# in order to change permissions; afterwards, the script is re-launched
# as the algorand user.
if [ "$(id -u)" = '0' ]; then
  chown -R algorand:algorand $INDEXER_DATA
  exec gosu algorand "$0" "$@"
fi

# copy config.yml override to data directory
if [[ -f /etc/algorand/indexer.yml ]]; then
  cp /etc/algorand/indexer.yml /data/indexer.yml
fi

# always run the algorand-indexer command
exec algorand-indexer "$@"
