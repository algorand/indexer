package postgresql

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/data"
	_ "github.com/algorand/indexer/idb/dummy"
	"github.com/algorand/indexer/plugins"
	testutil "github.com/algorand/indexer/util/test"
)

var pgsqlConstructor = &Constructor{}
var logger *logrus.Logger
var round = basics.Round(0)

func init() {
	logger, _ = test.NewNullLogger()
}

func TestExporterMetadata(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	meta := pgsqlExp.Metadata()
	assert.Equal(t, plugins.PluginType(plugins.Exporter), meta.Type())
	assert.Equal(t, postgresqlExporterMetadata.ExpName, meta.Name())
	assert.Equal(t, postgresqlExporterMetadata.ExpDescription, meta.Description())
	assert.Equal(t, postgresqlExporterMetadata.ExpDeprecated, meta.Deprecated())
}

func TestConnectDisconnectSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true\nconnection-string: ''")
	assert.NoError(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger))
	assert.NoError(t, pgsqlExp.Close())
}

func TestConnectUnmarshalFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("'")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger), "connect failure in unmarshalConfig")
}

func TestConnectDbFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger), "connection string is empty for postgres")
}

func TestConfigDefault(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default postgresql.ExporterConfig: %v", err)
	}
	assert.Equal(t, plugins.PluginConfig(expected), pgsqlExp.Config())
}

func TestReceiveInvalidBlock(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger))

	invalidBlock := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{},
		Payset:      transactions.Payset{},
		Certificate: &agreement.Certificate{},
		Delta:       nil,
	}
	expectedErr := fmt.Sprintf("receive got an invalid block: %#v", invalidBlock)
	assert.EqualError(t, pgsqlExp.Receive(invalidBlock), expectedErr)
}

func TestReceiveAddBlockSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger))

	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{},
		Payset:      transactions.Payset{},
		Certificate: &agreement.Certificate{},
		Delta:       &ledgercore.StateDelta{},
	}
	assert.NoError(t, pgsqlExp.Receive(block))
}

func TestPostgresqlExporterInit(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true")

	// genesis hash mismatch
	initProvider := testutil.MockedInitProvider(&round)
	initProvider.Genesis = &bookkeeping.Genesis{
		Network: "test",
	}
	err := pgsqlExp.Init(context.Background(), initProvider, cfg, logger)
	assert.Contains(t, err.Error(), "error importing genesis: genesis hash not matching")

	// incorrect round
	round = 1
	err = pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger)
	assert.Contains(t, err.Error(), "initializing block round 1 but next round to account is 0")
}
