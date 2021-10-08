package testing

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/require"
)

var testpg string

func init() {
	flag.StringVar(&testpg, "test-pg", "", "postgres connection string; resets the database")
	if testpg == "" {
		fmt.Println("SETTING TEST_PG")
		testpg = os.Getenv("TEST_PG")
		fmt.Println(testpg)
	}
}

// SetupPostgres starts a gnomock postgres DB then returns the database object,
// the connection string and a shutdown function.
func SetupPostgres(t *testing.T) (*pgxpool.Pool, string, func()) {
	if testpg != "" {
		// use non-docker Postgresql
		shutdownFunc := func() {
			// nothing to do, psql db setup/teardown is external
		}
		connStr := testpg

		db, err := pgxpool.Connect(context.Background(), connStr)
		require.NoError(t, err, "Error opening postgres connection")

		_, err = db.Exec(
			context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
		require.NoError(t, err)

		return db, connStr, shutdownFunc
	}

	p := postgres.Preset(
		postgres.WithVersion("12.5"),
		postgres.WithUser("gnomock", "gnomick"),
		postgres.WithDatabase("mydb"),
	)
	container, err := gnomock.Start(p)
	require.NoError(t, err, "Error starting gnomock")

	shutdownFunc := func() {
		err = gnomock.Stop(container)
		require.NoError(t, err, "Error stoping gnomock")
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s  dbname=%s sslmode=disable",
		container.Host, container.DefaultPort(),
		"gnomock", "gnomick", "mydb",
	)

	db, err := pgxpool.Connect(context.Background(), connStr)
	require.NoError(t, err, "Error opening postgres connection")

	return db, connStr, shutdownFunc
}
