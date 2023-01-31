package postgresql

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"

	"github.com/algorand/indexer/conduit/plugins"
	"github.com/algorand/indexer/conduit/plugins/exporters"
	"github.com/algorand/indexer/conduit/plugins/exporters/postgresql/util"
	"github.com/algorand/indexer/data"
	_ "github.com/algorand/indexer/idb/dummy"
	testutil "github.com/algorand/indexer/util/test"
)

var pgsqlConstructor = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &postgresqlExporter{}
})
var logger *logrus.Logger
var round = basics.Round(0)

func init() {
	logger, _ = test.NewNullLogger()
}

func TestExporterMetadata(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	meta := pgsqlExp.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestConnectDisconnectSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true\nconnection-string: ''")
	assert.NoError(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger))
	assert.NoError(t, pgsqlExp.Close())
}

func TestConnectUnmarshalFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("'")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger), "connect failure in unmarshalConfig")
}

func TestConnectDbFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger), "connection string is empty for postgres")
}

func TestConfigDefault(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	defaultConfig := &ExporterConfig{}
	expected, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("unable to Marshal default postgresql.ExporterConfig: %v", err)
	}
	assert.Equal(t, string(expected), pgsqlExp.Config())
}

func TestReceiveInvalidBlock(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger))

	invalidBlock := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{
			Round: 1,
		},
		Payset:      transactions.Payset{},
		Certificate: &agreement.Certificate{},
		Delta:       nil,
	}
	expectedErr := fmt.Sprintf("receive got an invalid block: %#v", invalidBlock)
	assert.EqualError(t, pgsqlExp.Receive(invalidBlock), expectedErr)
}

func TestReceiveAddBlockSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true")
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
	cfg := plugins.MakePluginConfig("test: true")

	// genesis hash mismatch
	initProvider := testutil.MockedInitProvider(&round)
	initProvider.Genesis = &sdk.Genesis{
		Network: "test",
	}
	err := pgsqlExp.Init(context.Background(), initProvider, cfg, logger)
	assert.Contains(t, err.Error(), "error importing genesis: genesis hash not matching")

	// incorrect round
	round = 1
	err = pgsqlExp.Init(context.Background(), testutil.MockedInitProvider(&round), cfg, logger)
	assert.Contains(t, err.Error(), "initializing block round 1 but next round to account is 0")
}

func TestUnmarshalConfigsContainingDeleteTask(t *testing.T) {
	// configured delete task
	pgsqlExp := postgresqlExporter{}
	cfg := ExporterConfig{
		ConnectionString: "",
		MaxConn:          0,
		Test:             true,
		Delete: util.PruneConfigurations{
			Rounds:   3000,
			Interval: 3,
		},
	}
	data, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.NoError(t, pgsqlExp.unmarhshalConfig(string(data)))
	assert.Equal(t, 3, int(pgsqlExp.cfg.Delete.Interval))
	assert.Equal(t, uint64(3000), pgsqlExp.cfg.Delete.Rounds)

	// delete task with fields default to 0
	pgsqlExp = postgresqlExporter{}
	cfg = ExporterConfig{
		ConnectionString: "",
		MaxConn:          0,
		Test:             true,
		Delete:           util.PruneConfigurations{},
	}
	data, err = yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.NoError(t, pgsqlExp.unmarhshalConfig(string(data)))
	assert.Equal(t, 0, int(pgsqlExp.cfg.Delete.Interval))
	assert.Equal(t, uint64(0), pgsqlExp.cfg.Delete.Rounds)

	// delete task with negative interval
	pgsqlExp = postgresqlExporter{}
	cfg = ExporterConfig{
		ConnectionString: "",
		MaxConn:          0,
		Test:             true,
		Delete: util.PruneConfigurations{
			Rounds:   1,
			Interval: -1,
		},
	}
	data, err = yaml.Marshal(cfg)
	assert.NoError(t, pgsqlExp.unmarhshalConfig(string(data)))
	assert.Equal(t, -1, int(pgsqlExp.cfg.Delete.Interval))

	// delete task with negative round
	pgsqlExp = postgresqlExporter{}
	cfgstr := "test: true\ndelete-task:\n  rounds: -1\n  interval: 2"
	assert.ErrorContains(t, pgsqlExp.unmarhshalConfig(cfgstr), "unmarshal errors")

}
