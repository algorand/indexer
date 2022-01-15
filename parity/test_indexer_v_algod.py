from pathlib import Path
import json
from textwrap import indent

from .json_diff import deep_diff, flatten_diff, report_diff

BEFORE_MINBALANCE = True

expected_overlap_diff_before_minbalance = {
    "definitions": {
        "Account": {
            "properties": {
                "sig-type": {
                    "description": [
                        "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig\n* or null if unknown",
                        "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig",
                    ]
                }
            },
            "required": [[None, "min-balance"]],
        },
        "ApplicationParams": {
            "properties": {
                "global-state-schema": {
                    "description": [
                        "[\\lsch\\] global schema",
                        "[\\gsch\\] global schema",
                    ]
                }
            },
            "required": [[None, "creator"]],
        },
        "TealValue": {
            "properties": {
                "type": {
                    "description": [
                        "\\[tt\\] value type.",
                        "\\[tt\\] value type. Value `1` refers to **bytes**, value `2` refers to **uint**",
                    ]
                }
            }
        },
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
expected_overlap_diff_after_minbalance = {
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

expected_overlap_report_before_minbalance = """
definitions.Account.properties.sig-type.description:"Indicates what type of signature is used by this account, must be one of:\\n* sig\\n* msig\\n* lsig\\n* or null if unknown"
                                                   :"Indicates what type of signature is used by this account, must be one of:\\n* sig\\n* msig\\n* lsig"
definitions.Account.required.0:null
                              :"min-balance"
definitions.ApplicationParams.properties.global-state-schema.description:"[\\\\lsch\\\\] global schema"
                                                                        :"[\\\\gsch\\\\] global schema"
definitions.ApplicationParams.required.0:null
                                        :"creator"
definitions.TealValue.properties.type.description:"\\\\[tt\\\\] value type."
                                                 :"\\\\[tt\\\\] value type. Value `1` refers to **bytes**, value `2` refers to **uint**"
parameters.limit.description:"Maximum number of results to return. There could be additional pages even if the limit is not reached."
                            :"Maximum number of results to return."
responses.ApplicationResponse.description:"(empty)"
                                         :"Application information"
responses.AssetResponse.description:"(empty)"
                                   :"Asset information"
responses.BlockResponse.description:"(empty)"
                                   :"Encoded block object."
""".strip()


def fancy_report(diff_json):
    return report_diff(diff_json, blank_diff_path=True, src="ALGOD", tgt="INDEXER")


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

    expected_diff = (
        expected_overlap_diff_before_minbalance
        if BEFORE_MINBALANCE
        else expected_overlap_diff_after_minbalance
    )
    diff_of_diffs = deep_diff(expected_diff, overlap_diff)
    assert diff_of_diffs is None, diff_of_diffs

    report = report_diff(expected_overlap_diff_before_minbalance)
    expected_report = expected_overlap_report_before_minbalance
    assert expected_report == report, "mods reports differ"

    diff_report = repo / "parity" / "indexer_algod_mods.txt"
    with open(diff_report, "w") as f:
        f.write(fancy_report(overlap_diff))

    # Additions - fields that have been introduced in indexer
    indexer_add_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="left"
    )
    diff_json = repo / "parity" / "indexer_algod_adds.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_add_json, indent=2, sort_keys=True))

    diff_report = repo / "parity" / "indexer_algod_adds.txt"
    with open(diff_report, "w") as f:
        f.write(fancy_report(indexer_add_json))

    # Removals - fields that have been deleted in indexer
    indexer_remove_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="right"
    )
    diff_json = repo / "parity" / "indexer_algod_removes.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_remove_json, indent=2, sort_keys=True))

    diff_report = repo / "parity" / "indexer_algod_removes.txt"
    with open(diff_report, "w") as f:
        f.write(fancy_report(indexer_remove_json))

    # Full Diff - anything that's different
    indexer_full_json = deep_diff(indexer, algod, exclude_keys=exclude, arraysets=True)
    diff_json = repo / "parity" / "indexer_algod_full_diff.json"
    with open(diff_json, "w") as f:
        f.write(json.dumps(indexer_full_json, indent=2, sort_keys=True))

    diff_report = repo / "parity" / "indexer_algod_full_diff.txt"
    with open(diff_report, "w") as f:
        f.write(fancy_report(indexer_full_json))
