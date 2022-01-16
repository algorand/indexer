from cmath import exp
from copy import deepcopy

from .json_diff import deep_diff, flatten_diff


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

    actual = deep_diff(real1, real2, extras_only="left", arraysets=True)
    expected = None
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    fb1 = {"FANG": [{"Facebook": {"price": 330}}]}
    fb2 = {"FANG": [{"Meta": {"price": 290}}]}
    actual = deep_diff(fb1, fb2)
    expected = {
        "FANG": [{"Facebook": [{"price": 330}, None], "Meta": [None, {"price": 290}]}]
    }
    assert expected == actual, f"expected: {expected} v. actual: {actual}"


def test_flatten_diff():
    actual = flatten_diff(None)
    expected = [":null"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(42)
    expected = [":42"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff("seventeen")
    expected = [':"seventeen"']
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(["one", "two"])
    expected = ['0:"one"', '1:"two"']
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(["one", None])
    expected = ['0:"one"', "1:null"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(["one", None, 42])
    expected = ['0:"one"', "1:null", "2:42"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(["one", ["three", None], 42])
    expected = ['0:"one"', '1:"three"', " :null", "2:42"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff(["one", [None, None], 42])  # 2nd level shouldn't be a diff!
    expected = ['0:"one"', "1.0:null", "1.1:null", "2:42"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    # 2nd level shouldn't be a diff!
    actual = flatten_diff(["one", [None, 1337, None], 42])
    expected = ['0:"one"', "1.0:null", "1.1:1337", "1.2:null", "2:42"]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    actual = flatten_diff([[["one", "two"]]])
    expected = ['0.0:"one"', '   :"two"']
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    diff = {
        "FANG": [
            {"Google": {"price": [2900, 2950]}},
        ]
    }
    actual = flatten_diff(diff)
    expected = [
        "FANG.0.Google.price:2900",
        "                   :2950",
    ]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    diff = {
        "FANG": [{"Facebook": [{"price": 330}, None], "Meta": [None, {"price": 290}]}]
    }
    actual = flatten_diff(diff)
    expected = [
        'FANG.0.Facebook:{"price":330}',
        "               :null",
        "FANG.0.Meta:null",
        '           :{"price":290}',
    ]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"

    diff = {
        "FANG": [
            {"Google": {"price": [2900, 2950]}},
            {"Facebook": [{"price": 330}, None], "Meta": [None, {"price": 290}]},
            {"IBM": {"price": [130, 125]}},
        ]
    }
    actual = flatten_diff(diff)
    expected = [
        "FANG.0.Google.price:2900",
        "                   :2950",
        'FANG.1.Facebook:{"price":330}',
        "               :null",
        "FANG.1.Meta:null",
        '           :{"price":290}',
        "FANG.2.IBM.price:130",
        "                :125",
    ]
    assert expected == actual, f"expected: {expected} v. actual: {actual}"
