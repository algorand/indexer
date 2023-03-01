# Filter Processor

This is used to filter transactions to include only the ones that you want. This may be useful for some deployments
which only require specific applications or accounts.

## any / all
One or more top-level operations should be provided.
* any: transactions are included if they match `any` of the nested sub expressions.
* all: transactions are included if they match `all` of the nested sub expressions.

If `any` and `all` are both provided, the transaction must pass both checks.

## Sub expressions

Parts of an expression:
* `tag`: the transaction field being considering.
* `expression-type`: The type of expression.
* `expression`: Input to the expression

### tag
The full path to a given field. Uses the messagepack encoded names of a canonical transaction. For example:
* `txn.snd` is the sender.
* `txn.amt` is the amount.

For information about the structure of transactions, refer to the [Transaction Structure](https://developer.algorand.org/docs/get-details/transactions/) documentation. For detail about individual fields, refer to the [Transaction Reference](https://developer.algorand.org/docs/get-details/transactions/transactions/) documentation.

**Note**: The "Apply Data" information is also available for filtering. These fields are not well documented. Advanced users can inspect raw transactions returned by algod to see what fields are available.

### expression-type

What type of expression to use for filtering the tag.
* `exact`: exact match for string values.
* `regex`:  applies regex rules to the matching.
* `less-than` applies numerical less than expression.
* `less-than-equal` applies numerical less than or equal expression.
* `greater-than` applies numerical greater than expression.
* `greater-than-equal` applies numerical greater than or equal expression.
* `equal` applies numerical equal expression.
* `not-equal` applies numerical not equal expression.

### expression

The input to the expression. A number or string depending on the expression type.

# Config
```yaml
processors:
  - name: filter_processor
    config:
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

