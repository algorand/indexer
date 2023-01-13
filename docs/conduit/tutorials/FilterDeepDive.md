## Filtering Transactions in Conduit

### Intro
Conduit provides individual documentation for each plugin in [docs/conduit/plugins](./plugins). However, the filter
processor in particular has a complex set of features which empower users to search for data within Transactions.
This document will go through some of those features in detail, their use cases, and show some examples.

### Logical Operators

The filter processor provides (at this time) two top level logical operators, `any` and `all`. These are used to match
"sub-expressions" specified in the filters. For any set of expressions, e1, e2, e3, ... `any(e1,e2,e3,...eN)` will return
`true` if there exists `eX` for `1 >= X <= N` where `eX` evaulates to `true`,
and `all(e1,e2,e3,...eN)` will return true if for every `X` from `1..N`, `eX` evaluates to `true`.

In simpler terms, `any` matches the transaction if at least one sub-expression matches, and `all` matches only if every
sub-expression matches.

### Sub-Expressions
So, what defines a sub-expression?

The sub-expression consists of 3 components.  
#### `tag`
The tag identifies the field to attempt to match. The fields derive their tags according to the
[official reference docs](https://developer.algorand.org/docs/get-details/transactions/transactions/).
You can also attempt to match against the `ApplyData`, although this is not officially supported or documented.
Users interested in this will need to consult the official
[go-algorand](https://github.com/algorand/go-algorand/blob/master/data/transactions/transaction.go#L104) repository to match tags.


For now, we programmatically generate these fields into a map located in the
[filter package](https://github.com/algorand/indexer/blob/develop/conduit/plugins/processors/filterprocessor/fields/generated_signed_txn_map.go),
though this is not guaranteed to be the case.


Example:
```yaml
- tag: 'txn.snd' # Matches the Transaction Sender
- tag: 'txn.apar.c' # Matches the Clawback address of the asset params
- tag: 'txn.amt' # Matches the amount of a payment transaction
```

#### `expression-type`
The expression type is a selection of one of the available methods for evaluating the expression. The current list of
types is 
* `exact`: exact match for string values.
* `regex`:  applies regex rules to the matching.
* `less-than` applies numerical less than expression.
* `less-than-equal` applies numerical less than or equal expression.
* `greater-than` applies numerical greater than expression.
* `greater-than-equal` applies numerical greater than or equal expression.
* `equal` applies numerical equal expression.
* `not-equal` applies numerical not equal expression.

You must use the proper expression type for the field your tag identifies based on the type of data stored in that field.
For example, do not use a numerical expression type on a string field such as address.


#### `expression`
The expression is the data against which each field will be compared. This must be compatible with the data type of
the expected field. For string fields you can also use the `regex` expression type to interpret the input of the
expression as a regex. 

### Examples

Find transactions w/ fee greater than 1000 microalgos
```yaml
- filters:
  - any:
    - tag: "txn.fee"
      expression-type: "greater-than"
      expression: "1000"
```

Find state proof transactions
```yaml
- filters:
  - any:
    - tag: "txn.type"
      expression-type: "exact"
      expression: "stpf"
```

Find transactions calling app, "MYAPPID"
```yaml
- filters:
  - all:
    - tag: "txn.type"
      expression-type: "exact"
      expression: "appl"
    - tag: "txn.apid"
      expression-type: "exact"
      expression: "MYAPPID"
```