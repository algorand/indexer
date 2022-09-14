from e2e_conduit.fixtures.plugin_fixture import PluginFixture


class Scenario:
    """Data class for conduit E2E test pipelines"""

    def __init__(
        self,
        name,
        importer: PluginFixture,
        processors: list[PluginFixture],
        exporter: PluginFixture,
    ):
        self.name = name
        self.importer = importer
        self.processors = processors
        self.exporter = exporter
        self.accumulated_config = {}
        self.conduit_dir = ""


scenarios = []
