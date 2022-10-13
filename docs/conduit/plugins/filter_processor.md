# Filter Processor

This is used to filter transactions to include only the ones that you want. This may be useful for some deployments
which only require specific applications or accounts.

## any / all
One or more top-level operations should be provided. 
* any: transactions are included if they match `any` of the nested sub expressions.
* all: transactions are included if they match `all` of the nested sub expressions.

If `any` and `all` are both provided, the transaction must pass both checks.

## Sub expressions

The sub expressions define a `tag` representing a field in the transaction. The full path to a given field should be provided, for example:
* `txn.snd` is the sender.
* `tsn.amt` is the amount.

For information about the structure of transactions, refer to the [Transaction Structure](https://developer.algorand.org/docs/get-details/transactions/) documentation. For detail about individual fields, refer to the [Transaction Reference](https://developer.algorand.org/docs/get-details/transactions/transactions/) documentation.

**Note**: The "Apply Data" information is also available for filtering. These fields are not well documented. Advanced users can inspect raw transactions returned by algod to see what fields are available.

# Config
```yaml
Processors:
  - Name: filter_processor
    Config:
      - filters:
          - any
              - tag:
                expression-type:
                expression:
              - tag:
                expression-type:
                expression:
          - all
              - tag:
              expression-type:
              expression:
              - tag:
                expression-type:
                expression:
```

