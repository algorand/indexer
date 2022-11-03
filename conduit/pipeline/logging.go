package pipeline

import (
	log "github.com/sirupsen/logrus"
)

// PluginLogFormatter formats the log message with special conduit tags
type PluginLogFormatter struct {
	Formatter *log.JSONFormatter
	Type      string
	Name      string
}

// Format allows this to be used as a logrus formatter
func (f PluginLogFormatter) Format(entry *log.Entry) ([]byte, error) {
	// Underscores force these to be in the front in order type -> name
	entry.Data["__type"] = f.Type
	entry.Data["_name"] = f.Name
	return f.Formatter.Format(entry)
}

func makePluginLogFormatter(pluginType string, pluginName string) PluginLogFormatter {
	return PluginLogFormatter{
		Formatter: &log.JSONFormatter{
			DisableHTMLEscape: true,
		},
		Type: pluginType,
		Name: pluginName,
	}
}
