# PostgreSQL Exporter

Write block data to a postgres database with the Indexer REST API schema.

## Connection string

We are using the [pgx](https://github.com/jackc/pgconn) database driver, which dictates the connection string format.

For most deployments, you can use the following format:
`host={url} port={port} user={user} password={password} dbname={db_name} sslmode={enable|disable}`

For additional details, refer to the [parsing documentation here](https://pkg.go.dev/github.com/jackc/pgx/v4/pgxpool@v4.11.0#ParseConfig).

# Config
```yaml
exporter:
  - name: postgresql
    config:
      - connection-string: "postgres connection string"
        max-conn: "connection pool setting, maximum active queries"
        test: "a boolean, when true a mock database is used"
```

