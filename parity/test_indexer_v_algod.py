from pathlib import Path
import json
from textwrap import indent

from .json_diff import deep_diff

expected_overlap_diff = {
    "definitions": {
        "Account": {
            "properties": {
                "sig-type": {
                    "description": [
                        "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig\n* or null if unknown",
                        "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig",
                    ]
                }
            }
        },
        "ApplicationParams": {"required": [[None, "creator"]]},
    },
    "parameters": {
        "limit": {
            "description": [
                "Maximum number of results to return. There could be additional pages even if the limit is not reached.",
                "Maximum number of results to return.",
            ]
        }
    },
    "responses": {
        "ApplicationResponse": {"description": ["(empty)", "Application information"]},
        "AssetResponse": {"description": ["(empty)", "Asset information"]},
        "BlockResponse": {"description": ["(empty)", "Encoded block object."]},
    },
}


def test_parity():
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
        "tags",
        "x-algorand-format",
        "x-go-name",
    ]
    repo = Path.cwd()
    indexer_json = repo / "api" / "indexer.oas2.json"
    algod_json = (
        repo
        / "third_party"
        / "go-algorand"
        / "daemon"
        / "algod"
        / "api"
        / "algod.oas2.json"
    )
    with open(indexer_json, "r") as f:
        indexer = json.loads(f.read())

    with open(algod_json, "r") as f:
        algod = json.loads(f.read())

    # Overlaps - existing fields that have been modified freom algod ---> indexer
    overlap_diff = deep_diff(
        indexer, algod, exclude_keys=exclude, overlaps_only=True, arraysets=True
    )
    diff_json = repo / "parity" / "indexer_algod_mods.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(overlap_diff, indent=2, sort_keys=True))

    diff_of_diffs = deep_diff(expected_overlap_diff, overlap_diff)
    assert diff_of_diffs is None, diff_of_diffs

    # Additions - fields that have been introduced in indexer
    indexer_add_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="left"
    )
    diff_json = repo / "parity" / "indexer_algod_adds.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_add_json, indent=2, sort_keys=True))

    # Removals - fields that have been deleted in indexer
    indexer_remove_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="right"
    )
    diff_json = repo / "parity" / "indexer_algod_removes.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_remove_json, indent=2, sort_keys=True))

    # Full Diff - anything that's different
    indexer_remove_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True
    )
    diff_json = repo / "parity" / "indexer_algod_full_diff.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_remove_json, indent=2, sort_keys=True))
