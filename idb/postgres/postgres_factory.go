package postgres

import (
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/idb"
)

type postgresFactory struct {
}

func (df postgresFactory) Name() string {
	return "postgres"
}

func (df postgresFactory) Build(config *pgxpool.Config, opts idb.IndexerDbOptions, log *log.Logger) (idb.IndexerDb, chan struct{}, error) {
	return OpenPostgres(config, opts, log)
}

func init() {
	idb.RegisterFactory("postgres", &postgresFactory{})
}
