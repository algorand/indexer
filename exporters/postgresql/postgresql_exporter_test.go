package postgresql

import (
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/data"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"gopkg.in/yaml.v3"

	_ "github.com/algorand/indexer/idb/dummy"
	"github.com/algorand/indexer/plugins"

	"github.com/stretchr/testify/assert"
)

var pgsqlConstructor = &Constructor{}
var logger *logrus.Logger

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
	assert.NoError(t, pgsqlExp.Init(cfg, logger))
	assert.NoError(t, pgsqlExp.Close())
}

func TestConnectUnmarshalFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("'")
	assert.ErrorContains(t, pgsqlExp.Init(cfg, logger), "connect failure in unmarshalConfig")
}

func TestConnectDbFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("")
	assert.ErrorContains(t, pgsqlExp.Init(cfg, logger), "connect failure constructing db, postgres:")
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

func TestDefaultRoundZero(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	assert.Equal(t, uint64(0), pgsqlExp.Round())
}

func TestHandleGenesis(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(cfg, logger))
	assert.NoError(t, pgsqlExp.HandleGenesis(bookkeeping.Genesis{}))
}

func TestReceiveInvalidBlock(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.PluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(cfg, logger))

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
	assert.NoError(t, pgsqlExp.Init(cfg, logger))

	block := data.BlockData{
		BlockHeader: bookkeeping.BlockHeader{},
		Payset:      transactions.Payset{},
		Certificate: &agreement.Certificate{},
		Delta:       &ledgercore.StateDelta{},
	}
	assert.NoError(t, pgsqlExp.Receive(block))
}

func TestUnmarshalConfigsContainingDeleteTask(t *testing.T) {
	// configured delete task
	pgsqlExp := postgresqlExporter{}
	cfg := "test: true\ndelete-task:\n  rounds: 3000\n  interval: 3\n  timeout: 5"
	assert.NoError(t, pgsqlExp.unmarhshalConfig(cfg))
	assert.Equal(t, 3, int(pgsqlExp.cfg.Delete.Interval))
	assert.Equal(t, uint64(3000), pgsqlExp.cfg.Delete.Rounds)
	assert.Equal(t, uint64(5), pgsqlExp.cfg.Delete.Timeout)

	// delete task with fields default to 0
	pgsqlExp = postgresqlExporter{}
	cfg = "test: true\n"
	assert.NoError(t, pgsqlExp.unmarhshalConfig(cfg))
	assert.Equal(t, 0, int(pgsqlExp.cfg.Delete.Interval))
	assert.Equal(t, uint64(0), pgsqlExp.cfg.Delete.Rounds)
	assert.Equal(t, uint64(0), pgsqlExp.cfg.Delete.Timeout)

	// delete task with negative round
	pgsqlExp = postgresqlExporter{}
	cfg = "test: true\ndelete-task:\n  rounds: -1\n  interval: 2"
	assert.ErrorContains(t, pgsqlExp.unmarhshalConfig(cfg), "unmarshal errors")

	// delete task with negative interval
	pgsqlExp = postgresqlExporter{}
	cfg = "test: true\ndelete-task:\n  interval: -1"
	assert.NoError(t, pgsqlExp.unmarhshalConfig(cfg))
	assert.Equal(t, -1, int(pgsqlExp.cfg.Delete.Interval))
}
