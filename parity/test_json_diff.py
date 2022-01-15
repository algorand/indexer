from cmath import exp
from copy import deepcopy

from json_diff import deep_diff


def test_deep_diff():
    d1 = {
        "dad": 55,
        "mom": 56,
    }
    d2 = {
        "mom": 55,
        "dad": 55,
    }
    actual = deep_diff(d1, d2)
    expected = {"mom": [56, 55]}
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = deep_diff(d1, deepcopy(d1))
    expected = None
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    mom_info = {
        "age": 56,
        "profession": "MD",
        "hobbies": ["ballet", "opera", {"football": "american"}, "racecar driving"],
    }
    d3 = {
        "dad": 55,
        "mom": mom_info,
    }
    actual = deep_diff(d1, d3)
    expected = {"mom": [56, mom_info]}
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    d4 = {
        "mom": mom_info,
    }
    actual = deep_diff(d3, d4)
    expected = {"dad": [55, None]}
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    d5 = {
        "dad": 55,
        "mom": {
            "age": 56,
            "profession": "Programmer",
            "hobbies": ["ballet", "opera", {"football": "british"}, "racecar driving"],
        },
    }
    actual = deep_diff(d3, d5)
    expected = {
        "mom": {
            "profession": ["MD", "Programmer"],
            "hobbies": [None, None, {"football": ["american", "british"]}, None],
        }
    }
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    a1 = ["hello", "world", {"I": "wish"}, "you", {"all": "the best"}]
    a2 = ["hello", "world", {"I": "wish"}, "you", {"all": "the very best"}]
    actual = deep_diff(a1, a2)
    expected = [None, None, None, None, {"all": ["the best", "the very best"]}]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    a3 = ["hello", "world", "I", "wish", "you", "good", "times"]
    a4 = ["world", "hello", "you", "good", "timesies", "wish"]
    actual = deep_diff(a3, a4, overlaps_only=True, arraysets=True)
    expected = [["I", None], ["times", None], [None, "timesies"]]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    s1 = ["alice", "bob", "cassie", "deandrea", "elbaz"]
    s2 = ["bob", "alice", "cassie", "deandrea", "elbaz", "farber"]
    actual = deep_diff(s1, s2)
    expected = [["alice", "bob"], ["bob", "alice"], None, None, None, [None, "farber"]]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = deep_diff(s1, s2, arraysets=True)
    expected = [[None, "farber"]]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    real1 = {
        "definitions": {
            "Account": {
                "properties": {
                    "sig-type": {
                        "description": "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig\n* or null if unknown"
                    }
                }
            }
        }
    }
    real2 = {
        "definitions": {
            "Account": {
                "properties": {
                    "sig-type": {
                        "description": "Indicates what type of signature is used by this account, must be one of:\n* sig\n* msig\n* lsig",
                    }
                }
            }
        }
    }
    expected = deepcopy(real2)
    expected["definitions"]["Account"]["properties"]["sig-type"]["description"] = [
        real1["definitions"]["Account"]["properties"]["sig-type"]["description"],
        real2["definitions"]["Account"]["properties"]["sig-type"]["description"],
    ]
    actual = deep_diff(real1, real2)
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    expected = None
    actual = deep_diff(real1, real2, extras_only="left", arraysets=True)
    assert expected == actual, f"expected: {expected} v. actual: {actual}"


test_deep_diff()
