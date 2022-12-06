# Block Evaluator

Enrich raw block data with a "State Delta" object which contains new account values. For example: new account balances, new application states, which assets have been created or deleted, etc.

The block evaluator computes the delta by maintaining a local ledger and evaluating each block locally. By default this data is written to the block evaluator plugin directory inside the indexer data directory.

State Delta's are required by some exporters.


## algod requirement

It is sometimes useful, or necessary to re-initialize the local ledger. To expedite that process a direct connection to algod is used.

This is configured by providing one of:
* `algod-data-dir`: this will automatically lookup the algod address and token. This is convenient when algod is installed locally to conduit.
* `algod-addr` and `algod-token`: explicitly provide the algod connection information.

## catchpoint

Fast catchup can be used for Mainnet, Testnet and Betanet. To use, lookup the most recent catchpoint for your network **without going over the desired round**. Use the following links:
* [Mainnet](https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/mainnet_catchpoints.txt)
* [Testnet](https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/testnet_catchpoints.txt)
* [Betanet](https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/betanet_catchpoints.txt)

For example, if you want to get **Mainnet** round `22212765`, you would refer to the Mainnet link above and find:
* `22210000#MZZIOYXYPPGNYRQHROXCPILIWIMQQRN7ZNLQJVM2QVSKT3QX6O4A`

# Config
```yaml
processors:
  - name: block_evaluator
    config:
      - ledger-dir: "override default local ledger location."
        algod-data-dir: "local algod data directory"
        algod-addr: "algod URL"
        algod-token: "algod token"
        catchpoint: "a catchpoint ID"
```

