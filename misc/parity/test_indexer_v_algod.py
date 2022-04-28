import atexit
import json
from pathlib import Path
from typing import List
import yaml

from git import Repo

from .json_diff import deep_diff, prettify_diff, select

NEW, OVERLAP, DROPPED, FULL = "new", "overlap", "dropped", "full"
DIFF_TYPES = [NEW, OVERLAP, DROPPED, FULL]

# These are the diff reports that will be run and compared/asserted against:
ASSERTIONS = [DROPPED, FULL]

# When non-empty, keep only:
PATH_INCLUDES = {"definitions": ["Account"]}

# Any diffs past one of the following keys in a path will be ignored:
PATH_KEY_EXCLUDES = []


REPO_DIR = Path.cwd()
INDEXER_SWGR = REPO_DIR / "api" / "indexer.oas2.json"

GOAL_DIR = REPO_DIR / "third_party" / "go-algorand"
ALGOD_SWGR = GOAL_DIR / "daemon" / "algod" / "api" / "algod.oas2.json"

REPORTS_DIR = REPO_DIR / "misc" / "parity" / "reports"


already_printed = False


def print_git_info_once():
    global already_printed
    if already_printed:
        return
    already_printed = True

    indexer = Repo(REPO_DIR)
    indexer_commit = indexer.git.rev_parse("HEAD")

    goal = Repo(GOAL_DIR)
    goal_commit = goal.git.rev_parse("HEAD")

    print(
        f"""Finished comparing:
    * Indexer Swagger {INDEXER_SWGR} for commit hash {indexer_commit}
    * Algod Swagger {ALGOD_SWGR} for commit hash {goal_commit}
"""
    )


def tsetup():
    atexit.register(print_git_info_once)

    with open(INDEXER_SWGR, "r") as f:
        indexer = json.loads(f.read())
        indexer = select(indexer, PATH_INCLUDES)

    with open(ALGOD_SWGR, "r") as f:
        algod = json.loads(f.read())
        algod = select(algod, PATH_INCLUDES)

    return PATH_KEY_EXCLUDES, indexer, algod


def get_report_path(diff_type, for_write=False):
    suffix = "_OUT" if for_write else ""
    yml_path = REPORTS_DIR / f"algod2indexer_{diff_type}{suffix}.yml"
    return yml_path


def save_yaml(diff, diff_type):
    yml_path = get_report_path(diff_type, for_write=True)
    with open(yml_path, "w") as f:
        f.write(yaml.dump(diff, indent=2, sort_keys=True, width=2000))
    print(f"\nsaved json diff to {yml_path}")


def yamlize(diff):
    def ddize(d):
        if isinstance(d, dict):
            return {k: ddize(v) for k, v in d.items()}
        if isinstance(d, list):
            return [ddize(x) for x in d]
        return d

    return ddize(prettify_diff(diff, src="ALGOD", tgt="INDEXER", value_limit=50))


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

    return yamlize(
        deep_diff(
            target,
            source,
            exclude_keys=excludes,
            overlaps_only=overlaps_only,
            extras_only=extras_only,
            arraysets=True,
        )
    )


def save_reports(*reports) -> None:
    """
    Generate a YAML report shoing differences between Algod's API and Indexer's API.

    Possible `reports` diff_types are:
    "overlap" - show only modifications to features that Algod and Indexer have in common
    "new" - focus on features added to Indexer and missing from Algod
    "dropped" (recommended) - focus on features that are present in Algod but dropped in Indexer
    "full" (recommended) - show all differences
    """
    excludes, indexer_swgr, algod_swgr = tsetup()

    for diff_type in reports:
        diff = generate_diff(algod_swgr, indexer_swgr, excludes, diff_type)
        save_yaml(diff, diff_type)


def test_parity(reports: List[str] = ASSERTIONS, save_new: bool = True):
    excludes, indexer_swgr, algod_swgr = tsetup()
    """
    For each report in reports:
       1. load the pre-existing yaml report into `old_diff`
       2. re-generate the equivalent report by comparing `algod_swgr` with `indexer_swgr`
       3. compute the `diff_of_diffs` between these two reports
       4. assert that there is no diff
    """

    if save_new:
        save_reports(*reports)

    for diff_type in reports:
        ypath = get_report_path(diff_type, for_write=False)
        with open(ypath, "r") as f:
            old_diff = yaml.safe_load(f)
        new_diff = generate_diff(algod_swgr, indexer_swgr, excludes, diff_type)

        diff_of_diffs = deep_diff(old_diff, new_diff)
        assert (
            diff_of_diffs is None
        ), f"""UNEXPECTED CHANGE IN {ypath}. Differences are:
{json.dumps(diff_of_diffs,indent=2)}
"""
