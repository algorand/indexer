import logging
import random
import string
import sys
import time

from e2e_conduit.fixtures.plugin_fixture import PluginFixture
from e2e_common.util import atexitrun, xrun

logger = logging.getLogger(__name__)


class PostgresqlExporter(PluginFixture):
    def __init__(self, max_conn=0):
        self.user = "algorand"
        self.password = "algorand"
        self.db_name = "e2e_db"
        # Should we have a random port here so that we can run multiple of these in parallel?
        self.port = "45432"
        self.container_name = ""
        self.max_conn = max_conn
        super().__init__()

    @property
    def name(self):
        return "postgresql"

    def setup(self, _):
        self.container_name = "".join(
            random.choice(string.ascii_lowercase) for i in range(10)
        )
        try:
            xrun(
                [
                    "docker",
                    "run",
                    "-d",
                    "--name",
                    self.container_name,
                    "-p",
                    f"{self.port}:5432",
                    "-e",
                    f"POSTGRES_PASSWORD={self.password}",
                    "-e",
                    f"POSTGRES_USER={self.user}",
                    "-e",
                    f"POSTGRES_DB={self.db_name}",
                    "postgres:13-alpine",
                ]
            )
            # Sleep 15 seconds to let postgresql start--otherwise conduit might fail on startup
            time.sleep(15)
            atexitrun(["docker", "kill", self.container_name])
        except Exception as exc:
            logger.error(f"docker postgres container startup failed: {exc}")
            sys.exit(1)

    def resolve_config_input(self):
        self.config_input = {
            "connection-string": f"host=localhost port={self.port} user={self.user} password={self.password} dbname={self.db_name} sslmode=disable",
            "max-conn": self.max_conn,
        }
