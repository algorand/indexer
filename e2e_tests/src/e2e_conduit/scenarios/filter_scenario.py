import e2e_conduit.fixtures.importers as importers
import e2e_conduit.fixtures.processors as processors
import e2e_conduit.fixtures.exporters as exporters
from e2e_conduit.scenarios import Scenario, scenarios


def app_filter_indexer_scenario(sourcenet):
    return Scenario(
        "app_filter_indexer_scenario",
        importer=importers.FollowerAlgodImporter(sourcenet),
        processors=[
            processors.FilterProcessor([
                {"any": [
                    {"tag": "txn.type",
                     "expression-type": "equal",
                     "expression": "appl"
                    }
                ]}
            ]),
        ],
        exporter=exporters.PostgresqlExporter(),
    )


def pay_filter_indexer_scenario(sourcenet):
    return Scenario(
        "pay_filter_indexer_scenario",
        importer=importers.FollowerAlgodImporter(sourcenet),
        processors=[
            processors.FilterProcessor([
                {"any": [
                    {"tag": "txn.type",
                     "expression-type": "equal",
                     "expression": "pay"
                    }
                ]}
            ]),
        ],
        exporter=exporters.PostgresqlExporter(),
    )
