package testing

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
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
		newDBName := strings.ToLower(t.Name())
		// use non-docker Postgresql
		connStr := testpg

		db, err := pgxpool.Connect(context.Background(), connStr)
		require.NoError(t, err, "Error opening postgres connection")

		_, err = db.Exec(
			context.Background(), fmt.Sprintf(`CREATE DATABASE %s;`, newDBName))
		//_, err = db.Exec(
		//	context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
		require.NoError(t, err)

		var connStrPartsNew []string
		parts := strings.Split(connStr, " ")
		for _, part := range parts {
			if strings.HasPrefix(part, "dbname") {
				connStrPartsNew = append(connStrPartsNew, fmt.Sprintf("dbname=%s", newDBName))
			} else {
				connStrPartsNew = append(connStrPartsNew, part)
			}
		}

		dbNew, err := pgxpool.Connect(context.Background(), strings.Join(connStrPartsNew, " "))
		require.NoError(t, err, "Error opening postgres connection to new database")

		shutdownFunc := func() {
			// nothing to do, psql db setup/teardown is external
			_, err = db.Exec(
				context.Background(), fmt.Sprintf(`DROP DATABASE %s`, newDBName))
			db.Close()
			dbNew.Close()
		}
		return dbNew, connStr, shutdownFunc
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
