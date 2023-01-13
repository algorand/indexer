class PluginFixture:
    def __init__(self):
        self.config_input = {}
        self.config_output = {}

    @property
    def name(self):
        raise NotImplementedError

    def resolve_config(self):
        self.resolve_config_input()
        self.resolve_config_output()

    def resolve_config_input(self):
        pass

    def resolve_config_output(self):
        pass

    def setup(self, accumulated_config):
        raise NotImplementedError
