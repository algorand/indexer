from pathlib import Path
import json
from textwrap import indent
import yaml

from .json_diff import deep_diff, report_diff, diff_summary, prettify_diff

BEFORE_MINBALANCE = True
MODELS_ONLY = True


def fancy_report(diff_json):
    return report_diff(
        diff_json,
        blank_diff_path=True,
        src="ALGOD",
        tgt="INDEXER",
        spacer="-" * 30 + "{}" + "-" * 30,
        extra_lines=2,
        must_be_even=True,
    )


def generate_report(folder, base_name, diff, summary=True):
    def ddize(d):
        if isinstance(d, dict):
            return {k: ddize(v) for k, v in d.items()}
        if isinstance(d, list):
            return [ddize(x) for x in d]
        return d

    diff_path = folder / (base_name + "_diff.json")
    with open(diff_path, "w") as f:
        f.write(json.dumps(diff, indent=2, sort_keys=True))
    print(f"\nsaved json diff to {diff_path}")

    pretty = ddize(prettify_diff(diff, src="ALGOD", tgt="INDEXER", value_limit=30))
    yml_path = folder / (base_name + "_diff.yml")
    with open(yml_path, "w") as f:
        f.write(yaml.dump(pretty, indent=2, sort_keys=True, width=2000))
    print(f"\nsaved json diff to {diff_path}")

    report_path = folder / (base_name + "_human.txt")
    report, num_diffs = fancy_report(diff)
    with open(report_path, "w") as f:
        f.write(report)
    print(f"\nsaved report with {num_diffs:.0f} diffs to {report_path}")

    if summary:
        summary_path = folder / (base_name + "_summary.txt")
        spacer = "_" * 20 + "{0:^30}" + "_" * 20
        summary, summary_size = diff_summary(
            diff, src="ALGOD", tgt="INDEXER", spacer=spacer
        )
        with open(summary_path, "w") as f:
            f.write(summary)
        print(f"\nsaved summary of size {summary_size:.0f} {summary_path}")


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
    reporting = repo / "parity" / "reports"

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
        if MODELS_ONLY:
            indexer = indexer["definitions"]

    with open(algod_json, "r") as f:
        algod = json.loads(f.read())
        if MODELS_ONLY:
            algod = algod["definitions"]

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
    if MODELS_ONLY:
        expected_diff = expected_diff["definitions"]

    diff_of_diffs = deep_diff(expected_diff, overlap_diff)
    assert diff_of_diffs is None, diff_of_diffs

    generate_report(reporting, "algod2indexer_mods", overlap_diff)

    # Additions - fields that have been introduced in indexer
    indexer_add_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="left"
    )
    generate_report(reporting, "algod2indexer_add", indexer_add_json, summary=False)

    # Removals - fields that have been deleted in indexer
    indexer_remove_json = deep_diff(
        indexer, algod, exclude_keys=exclude, arraysets=True, extras_only="right"
    )
    generate_report(
        reporting, "algod2indexer_remove", indexer_remove_json, summary=False
    )

    # Full Diff - anything that's different
    indexer_full_json = deep_diff(indexer, algod, exclude_keys=exclude, arraysets=True)
    generate_report(reporting, "algod2indexer_all", indexer_full_json)
