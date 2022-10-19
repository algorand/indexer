# Algod Importer

Fetch blocks one by one from the [algod REST API](https://developer.algorand.org/docs/rest-apis/algod/v2/). The node must be configured as an archival node in order to
provide old blocks.

Block data from the Algod REST API contains the block header, transactions, and a vote certificate.

# Config
```yaml
importer:
    name: algod
    config:
      - netaddr: "algod URL"
        token: "algod REST API token"
```

