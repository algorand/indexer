from e2e_conduit.fixtures.plugin_fixture import PluginFixture


class BlockEvaluator(PluginFixture):
    def __init__(self):
        self.name = "block_evaluator"
        super().__init__()

    def setup(
        self, algod_data_dir, algod_token, algod_net, indexer_data_dir, catchpoint=""
    ):
        self.algod_data_dir = algod_data_dir
        self.algod_token = algod_token
        self.algod_net = algod_net
        self.indexer_data_dir = indexer_data_dir
        self.catchpoint = catchpoint

    def resolve_config_input(self):
        self.config_input = {
            "catchpoint": self.catchpoint,
            "indexer-data-dir": self.indexer_data_dir,
            "algod-data-dir": self.algod_data_dir,
            "algod-token": self.algod_token,
            "algod-addr": self.algod_net,
        }
