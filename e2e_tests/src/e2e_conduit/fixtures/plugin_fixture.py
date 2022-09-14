class PluginFixture:
    def __init__(self):
        self.config_input = {}
        self.config_output = {}

    @property
    def name(self):
        raise NotImplemented

    def resolve_config_input(self):
        return self.config_input

    def resolve_config_output(self):
        return self.config_output

    def setup(self, accumulated_config):
        raise NotImplemented
