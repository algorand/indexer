import e2e_conduit.fixtures.importers as importers
import e2e_conduit.fixtures.processors as processors
import e2e_conduit.fixtures.exporters as exporters
from e2e_conduit.scenarios import Scenario


def sync_indexer_scenario(sourcenet):
    return Scenario(
        "sync_indexer_scenario",
        importer=importers.SyncAlgodImporter(sourcenet),
        processors=[
            processors.BlockEvaluator(),
        ],
        exporter=exporters.PostgresqlExporter(),
    )
