# PostgeSQL Custom Indexes

Indexer ships with a minimal index set in order to process blocks, and return transactions for accounts. More advanced queries and filters require customization of the indexes.

These must be configured manually on the database on an as-needed basis. [The PostgreSQL documentation should be used for more information.](https://www.postgresql.org/docs/13/indexes.html)

## Examples

In these examples `CONCURRENTLY` is used to create the index in the background on a running database.

### Transaction by asset id

```
CREATE INDEX CONCURRENTLY IF NOT EXISTS txn_asset ON txn (asset, round, intra);
```


### Query by rekey address

```
CREATE INDEX CONCURRENTLY IF NOT EXISTS account_by_spending_key ON account((account_data->>'spend'))
```

### Query all asset balances

```
CREATE INDEX CONCURRENTLY IF NOT EXISTS account_asset_asset ON account_asset (assetid, addr ASC);
```
