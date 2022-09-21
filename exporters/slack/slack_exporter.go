package slack

import (
	"fmt"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/indexer/data"
	"github.com/algorand/indexer/exporters"
	"github.com/algorand/indexer/plugins"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"gopkg.in/yaml.v3"
)

type slackExporter struct {
	round  uint64
	cfg    ExporterConfig
	logger *logrus.Logger
}

var slackExporterMetadata exporters.ExporterMetadata = exporters.ExporterMetadata{
	ExpName:        "slack",
	ExpDescription: "Sends slack messages with transaction information",
	ExpDeprecated:  false,
}

// Constructor is the ExporterConstructor implementation for an Exporter
type Constructor struct{}

// New initializes an Exporter
func (c *Constructor) New() exporters.Exporter {
	return &slackExporter{}
}

// Metadata returns the Exporter's Metadata object
func (exp *slackExporter) Metadata() exporters.ExporterMetadata {
	return slackExporterMetadata
}

// Init sets up the slack client
func (exp *slackExporter) Init(cfg plugins.PluginConfig, logger *logrus.Logger) error {
	exp.logger = logger
	err := yaml.Unmarshal([]byte(cfg), &exp.cfg)
	if err != nil {
		return fmt.Errorf("init failure in unmarshal config: %v", err)
	}
	return nil
}

// Config returns the unmarshaled config object
func (exp *slackExporter) Config() plugins.PluginConfig {
	ret, _ := yaml.Marshal(exp.cfg)
	return plugins.PluginConfig(ret)
}

// Close terminates connections
func (exp *slackExporter) Close() error {
	return nil
}

// Receive is the main handler function for blocks
func (exp *slackExporter) Receive(exportData data.BlockData) error {
	for _, txn := range exportData.Payset {
		exp.logger.Infof("got %v txns", len(exportData.Payset))
		for _, webhook := range exp.cfg.Webhooks {
			exp.logger.Infof("Sending txn message to webhook: %v", webhook)
			err := slack.PostWebhook(webhook, &slack.WebhookMessage{
				Username: "Conduit Slackbot",
				Text: fmt.Sprintf(
					"Transaction:\nSender: %v\nReceiver: %v\nAmount: %v\nNote: %v\n",
					txn.Txn.AssetSender,
					txn.Txn.AssetReceiver,
					txn.Txn.AssetAmount,
					string(txn.Txn.Note),
				),
			})
			if err != nil {
				exp.logger.Errorf("failed to send webhook message: %v", err)
				return err
			}
		}
	}
	exp.round = exportData.Round() + 1
	return nil
}

// HandleGenesis provides the opportunity to store initial chain state
func (exp *slackExporter) HandleGenesis(_ bookkeeping.Genesis) error {
	return nil
}

// Round should return the round number of the next expected round that should be provided to the Exporter
func (exp *slackExporter) Round() uint64 {
	return exp.round
}

func init() {
	// In order to provide a Constructor to the exporter_factory, we register our Exporter in the init block.
	// To load this Exporter into the factory, simply import the package.
	exporters.RegisterExporter(slackExporterMetadata.ExpName, &Constructor{})
}
