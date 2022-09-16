package postgresql

import "github.com/algorand/indexer/exporters/util"

// serde for converting an ExporterConfig to/from a PostgresqlExporterConfig

// ExporterConfig specific to the postgresql exporter
type ExporterConfig struct {
	// Pgsql connection string
	// See https://github.com/jackc/pgconn for more details
	ConnectionString string `yaml:"connection-string"`
	// Maximum connection number for connection pool
	// This means the total number of active queries that can be running
	// concurrently can never be more than this
	MaxConn uint32 `yaml:"max-conn"`
	// The test flag will replace an actual DB connection being created via the connection string,
	// with a mock DB for unit testing.
	Test bool `yaml:"test"`
	// the configuration for data pruning
	Delete util.PruneConfigurations `yaml:"delete-task"`
}
