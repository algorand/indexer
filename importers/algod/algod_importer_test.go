package algodimporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"testing"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/protocol"
	"github.com/algorand/go-algorand/rpcs"
	"github.com/algorand/go-codec/codec"
	"github.com/algorand/indexer/importers"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var (
	logger       *logrus.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	s            plugins.PluginConfig
	testImporter importers.Importer
)

func init() {
	logger, _ = test.NewNullLogger()
	ctx, cancel = context.WithCancel(context.Background())
	s = ""
}

func MockAlgodServerReturnsEmptyBlock() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rnd, _ := strconv.Atoi(path.Base(r.URL.Path))
		blk := rpcs.EncodedBlockCert{Block: bookkeeping.Block{BlockHeader: bookkeeping.BlockHeader{Round: basics.Round(rnd)}}}
		var blockbytes []byte
		w.WriteHeader(http.StatusOK)
		response := struct {
			Block bookkeeping.Block `codec:"block"`
		}{
			Block: blk.Block,
		}
		enc := codec.NewEncoderBytes(&blockbytes, protocol.CodecHandle)
		enc.Encode(response)
		w.Write(blockbytes)
	}))
}

func TestImporterorterMetadata(t *testing.T) {
	testImporter = New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Type(), algodImporterMetadata.Type())
	assert.Equal(t, metadata.Name(), algodImporterMetadata.Name())
	assert.Equal(t, metadata.Description(), algodImporterMetadata.Description())
	assert.Equal(t, metadata.Deprecated(), algodImporterMetadata.Deprecated())
}

func TestCloseSuccess(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, s, logger)
	assert.NoError(t, err)
	err = testImporter.Close()
	assert.NoError(t, err)
}

func TestInitSuccess(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, s, logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)
	testImporter.Close()
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, "`", logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
	testImporter.Close()
}

func TestConfigDefault(t *testing.T) {
	testImporter = New()
	expected, err := yaml.Marshal(&ImporterConfig{})
	if err != nil {
		t.Fatalf("unable to Marshal default algodimporter.ImporterConfig: %v", err)
	}
	assert.Equal(t, plugins.PluginConfig(expected), testImporter.Config())
}

func TestGetBlockFailure(t *testing.T) {
	testImporter = New()
	_, err := testImporter.Init(ctx, s, logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	blk, err := testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	assert.True(t, blk.Empty())
}

func TestGetBlockSuccess(t *testing.T) {
	ts := MockAlgodServerReturnsEmptyBlock()
	testImporter = New()
	_, err := testImporter.Init(ctx, plugins.PluginConfig("netaddr: "+ts.URL), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	downloadedBlk, err := testImporter.GetBlock(uint64(10))
	assert.NoError(t, err)
	assert.Equal(t, downloadedBlk.Round(), uint64(10))
	assert.True(t, downloadedBlk.Empty())
	cancel()
}
