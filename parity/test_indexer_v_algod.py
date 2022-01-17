from pathlib import Path
import json
from typing import List
import yaml

from .json_diff import deep_diff, prettify_diff

NEW, OVERLAP, DROPPED, FULL = "new", "overlap", "dropped", "full"
DIFF_TYPES = [NEW, OVERLAP, DROPPED, FULL]

# These are the diff reports that will be run and compared/asserted against:
ASSERTIONS = [DROPPED, FULL]

REPO_DIR = Path.cwd()
GOAL_DIR = REPO_DIR / "third_party" / "go-algorand"
REPORTS_DIR = REPO_DIR / "parity" / "reports"


def tsetup(models_only):
    exclude = [
        "basePath",
        "consumes",
        "host",
        "info",
        "paths",
        "produces",
        "security",
        "securityDefinitions",
        "schemes",
        "diff_types",
        "x-algorand-format",
        "x-go-name",
    ]

    indexer = REPO_DIR / "api" / "indexer.oas2.json"
    with open(indexer, "r") as f:
        indexer = json.loads(f.read())
        if models_only:
            indexer = indexer["definitions"]

    algod = GOAL_DIR / "daemon" / "algod" / "api" / "algod.oas2.json"
    with open(algod, "r") as f:
        algod = json.loads(f.read())
        if models_only:
            algod = algod["definitions"]

    return exclude, indexer, algod


def get_report_path(diff_type, for_write=False):
    suffix = "_OUT" if for_write else ""
    yml_path = REPORTS_DIR / f"algod2indexer_{diff_type}{suffix}.yml"
    return yml_path


def save_yaml(diff, diff_type):
    pretty = yamlize(diff)
    yml_path = get_report_path(diff_type, for_write=True)
    with open(yml_path, "w") as f:
        f.write(yaml.dump(pretty, indent=2, sort_keys=True, width=2000))
    print(f"\nsaved json diff to {yml_path}")


def yamlize(diff):
    def ddize(d):
        if isinstance(d, dict):
            return {k: ddize(v) for k, v in d.items()}
        if isinstance(d, list):
            return [ddize(x) for x in d]
        return d

    return ddize(prettify_diff(diff, src="ALGOD", tgt="INDEXER", value_limit=30))


def generate_diff(source, target, excludes, diff_type):
    assert (
        diff_type in DIFF_TYPES
    ), f"Unrecognized diff_type [{diff_type}] not in {DIFF_TYPES}"

    if diff_type == OVERLAP:
        # Overlaps - existing fields that have been modified freom algod ---> indexer
        overlaps_only = True
        extras_only = None
    elif diff_type == NEW:
        # Additions - fields that have been introduced in indexer
        overlaps_only = False
        extras_only = "left"
    elif diff_type == DROPPED:
        # Removals - fields that have been deleted in indexer
        overlaps_only = False
        extras_only = "right"
    else:
        # Full Diff - anything that's different
        assert diff_type == FULL
        overlaps_only = False
        extras_only = None

    return deep_diff(
        target,
        source,
        exclude_keys=excludes,
        overlaps_only=overlaps_only,
        extras_only=extras_only,
        arraysets=True,
    )


def save_reports(*reports, models_only: bool = True) -> None:
    """
    Generate a YAML report shoing differences between Algod's API and Indexer's API.

    Possible `reports` diff_types are:
    "overlap" - show only modifications to features that Algod and Indexer have in common
    "new" - focus on features added to Indexer and missing from Algod
    "dropped" (recommended) - focus on features that are present in Algod but dropped in Indexer
    "full" (recommended) - show all differences

    `models_only` - when True (recommended), trim down the Swaggers to only the `definitions`
    """
    excludes, indexer_swgr, algod_swgr = tsetup(models_only)

    for diff_type in reports:
        diff = generate_diff(algod_swgr, indexer_swgr, excludes, diff_type)
        save_yaml(diff, diff_type)


def test_parity(
    reports: List[str] = ASSERTIONS, models_only: bool = True, save_new: bool = True
):
    excludes, indexer_swgr, algod_swgr = tsetup(models_only)

    for diff_type in reports:
        ypath = get_report_path(diff_type, for_write=False)
        with open(ypath, "r") as f:
            old_diff = yaml.safe_load(f)
        new_diff = yamlize(generate_diff(algod_swgr, indexer_swgr, excludes, diff_type))

        diff_of_diffs = deep_diff(old_diff, new_diff)
        assert (
            diff_of_diffs is None
        ), f"""UNEXPECTED CHANGE IN {ypath}. Differences are:
{json.dumps(diff_of_diffs,indent=2)}
"""

    if save_new:
        save_reports(*reports, models_only=models_only)
