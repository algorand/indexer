import logging

from e2e_conduit.fixtures.plugin_fixture import PluginFixture

logger = logging.getLogger(__name__)


class BlockEvaluator(PluginFixture):
    def __init__(self, catchpoint=""):
        self.catchpoint = catchpoint
        self.algod_data_dir = None
        self.algod_token = None
        self.algod_net = None
        self.indexer_data_dir = None
        super().__init__()

    @property
    def name(self):
        return "block_evaluator"

    def setup(self, accumulated_config):
        try:
            self.algod_data_dir = accumulated_config["algod_data_dir"]
            self.algod_token = accumulated_config["algod_token"]
            self.algod_net = accumulated_config["algod_net"]
            self.indexer_data_dir = accumulated_config["conduit_dir"]
        except KeyError as exc:
            logger.error(
                f"BlockEvaluator must be provided with the proper config values {exc}"
            )
            raise

    def resolve_config_input(self):
        self.config_input = {
            "catchpoint": self.catchpoint,
            "data-dir": self.indexer_data_dir,
            "algod-data-dir": self.algod_data_dir,
            "algod-token": self.algod_token,
            "algod-addr": self.algod_net,
        }
