package postgres

import (
	"database/sql"
	"fmt"
	"github.com/algorand/indexer/idb"
	"github.com/stretchr/testify/require"
	"log"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/algorand/indexer/importer"
)


var (
	db *sql.DB

	user     = "postgres"
	password = "secret"
	database = "postgres"
)

func mustGetRandomPort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("Failed to get a port.")
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if err != nil {
		log.Fatal("Failed to get a port.")
		os.Exit(1)
	}
	return port
}

func initDB() (cleaner func()) {
	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	port := strconv.Itoa(mustGetRandomPort())

	opts := dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "12.3",
		Env: []string{
			"POSTGRES_USER=" + user,
			"POSTGRES_PASSWORD=" + password,
			"POSTGRES_DB=" + database,
		},
		ExposedPorts: []string{"5432"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5432": {
				{HostIP: "0.0.0.0", HostPort: port},
			},
		},
	}

	resource, err := pool.RunWithOptions(&opts)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err.Error())
	}

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", user, password, port, database)
	fmt.Printf("Connecting to posgres at: '%s'", dsn)
	err = pool.Retry(func() error {
		var err error
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			return err
		}
		return db.Ping()
	})
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	fmt.Println("DB Created")

	return func() {
		// You can't defer this because os.Exit doesn't care for defer
		if err := pool.Purge(resource); err != nil {
			log.Fatalf("Could not purge resource: %s", err)
		}
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("RUN_DOCKER_TESTS") == "" {
		fmt.Println("Skipping docker tests.\nEnable with environment variable 'RUN_DOCKER_TESTS=true'")
		return
	}

	cleaner := initDB()

	code := m.Run()

	cleaner()

	os.Exit(code)
}

func TestSomething(t *testing.T) {

	fmt.Println("Running the test?!")
	pdb, err := openPostgres(db, idb.IndexerDbOptions{
		ReadOnly: false,
	})

	h := importer.ImportHelper{
		GenesisJsonPath: "/home/will/algorand/indexer/foo/algod/genesis.json",
		NumRoundsLimit:  0,
		BlockFileLimit:  0,
	}
	h.Import(pdb, []string{"/home/will/algorand/indexer/foo/blocktars/*"})


	health, err := pdb.Health()
	require.NoError(t, err, "Failed to open postgres")
	fmt.Println(health)

	rnd, err := pdb.GetMaxRound()
	require.NoError(t, err, "Failed to get max round")
	fmt.Printf("Max round: %d\n", rnd)
	require.NoError(t, err, "Failed to get max round")
	fmt.Printf("Max round: %d\n", rnd)
}

type fakedb struct {
}