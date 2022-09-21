package slack

type ExporterConfig struct {
	// A list of webhooks. Each of which will have a POST request sent to it w/ a constructed Slack message
	// relating to each Transaction sent to the exporter.
	Webhooks []string `yaml:"webhooks"`
	// API Token for the Slack Client
	Token string `yaml:"token"`
}
