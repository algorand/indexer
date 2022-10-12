package conduit

import (
	"github.com/algorand/indexer/loggers"
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestPluginLogFormatter_Format tests the output of the formatter while pondering philosophy
func TestPluginLogFormatter_Format(t *testing.T) {

	pluginType := "A Question"
	pluginName := "What's in a name?"

	pluginFormatter := MakePluginLogFormatter(pluginType, pluginName)

	lMgr := loggers.MakeLoggerManager(os.Stdout)
	l := lMgr.MakeLogger()

	entry := &log.Entry{
		Time:    time.Time{},
		Level:   log.InfoLevel,
		Message: "That which we call a rose by any other name would smell just as sweet.",
		Data:    log.Fields{},
		Logger:  l.Logger,
	}

	bytes, err := pluginFormatter.Format(entry)
	assert.Nil(t, err)
	str := string(bytes)
	assert.Equal(t, str, "{\"__type\":\"A Question\",\"_name\":\"What's in a name?\",\"level\":\"info\",\"msg\":\"That which we call a rose by any other name would smell just as sweet.\",\"time\":\"0001-01-01T00:00:00Z\"}\n")

}
