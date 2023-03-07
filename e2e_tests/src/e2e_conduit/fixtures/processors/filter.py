from e2e_conduit.fixtures.plugin_fixture import PluginFixture


''' FilterProcessor is a simple passthrough for the `filter_processor`
    which sets the scenario's filters to the initialized value
'''
class FilterProcessor(PluginFixture):
    def __init__(self, filters, search_inner=True):
        self.filters = filters
        self.search_inner = search_inner
        super().__init__()

    @property
    def name(self):
        return "filter_processor"

    def resolve_config_input(self):
        self.config_input["filters"] = self.filters
        self.config_input["search-inner"] = self.search_inner

    def resolve_config_output(self):
        pass

    def setup(self, accumulated_config):
        pass
