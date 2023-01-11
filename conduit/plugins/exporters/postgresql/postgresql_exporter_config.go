package postgresql

//go:generate conduit-docs ../../../../conduit-docs/

import (
	"github.com/algorand/indexer/conduit/plugins/exporters/postgresql/util"
)

//Name: conduit_exporters_postgresql

// serde for converting an ExporterConfig to/from a PostgresqlExporterConfig

// ExporterConfig specific to the postgresql exporter
type ExporterConfig struct {
	/* <code>connectionstring</code> is the Postgresql connection string<br/>
	See https://github.com/jackc/pgconn for more details
	*/
	ConnectionString string `yaml:"connection-string"`
	/* <code>max-conn</code> specifies the maximum connection number for the connection pool.<br/>
	This means the total number of active queries that can be running concurrently can never be more than this.
	*/
	MaxConn uint32 `yaml:"max-conn"`
	/* <code>test</code> will replace an actual DB connection being created via the connection string,
	with a mock DB for unit testing.
	*/
	Test bool `yaml:"test"`
	// <code>delete-task</code> is the configuration for data pruning.
	Delete util.PruneConfigurations `yaml:"delete-task"`
}
