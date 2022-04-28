from collections import OrderedDict
from collections.abc import Hashable
from copy import deepcopy
import json
from typing import Iterable, List, Union


L, R = "left", "right"


def deep_diff(
    x: Union[dict, list],
    y: Union[dict, list],
    exclude_keys: List[str] = [],
    overlaps_only: bool = False,
    extras_only: Union[L, R, None] = None,
    arraysets: bool = False,
) -> Union[dict, list, None]:
    """
    Take the deep diff of JSON-like dictionaries
    """
    senseless = "it doesn't make sense to "
    if overlaps_only:
        assert (
            arraysets
        ), f"{senseless}diff overlaps only when not considering arrays as sets"
    if extras_only:
        assert (
            arraysets
        ), f"{senseless}have extras_only={extras_only} when not considering arrays as sets"
        assert (
            not overlaps_only
        ), f"{senseless}have extras_only={extras_only} when diffing overlaps only"

    right_extras = extras_only == R
    left_extras = extras_only == L

    def dd(x, y):
        if x == y:
            return None

        # awkward, but handles subclasses of dict/list:
        if not (
            isinstance(x, (list, dict))
            and (isinstance(x, type(y)) or isinstance(y, type(x)))
        ):
            return [x, y] if not extras_only else None

        if isinstance(x, dict):
            d = type(x)()  # handles OrderedDict's as well
            for k in x.keys() ^ y.keys():
                if k in exclude_keys or overlaps_only:
                    continue
                if (k in x and right_extras) or (k in y and left_extras):
                    continue
                d[k] = [deepcopy(x[k]), None] if k in x else [None, deepcopy(y[k])]

            for k in x.keys() & y.keys():
                if k in exclude_keys:
                    continue

                next_d = dd(x[k], y[k])
                if next_d is None:
                    continue

                d[k] = next_d

            return d if d else None

        # assume a list:
        m, n = len(x), len(y)
        if not arraysets:
            d = [None] * max(m, n)
            flipped = False
            if m > n:
                flipped = True
                x, y = y, x

            for i, x_val in enumerate(x):
                d[i] = dd(y[i], x_val) if flipped else dd(x_val, y[i])

            if not overlaps_only:
                for i in range(m, n):
                    d[i] = [y[i], None] if flipped else [None, y[i]]
        else:  # will raise error if contains a non-hashable element
            sx = set(hashablize(x))
            sy = set(hashablize(y))
            if extras_only:
                d = list(sx - sy) if left_extras else list(sy - sx)
            elif overlaps_only:
                ox, oy = sorted(x), sorted(y)
                d = []
                for e in ox:
                    if e not in oy:
                        d.append([e, None])
                for e in oy:
                    if e not in ox:
                        d.append([None, e])
            else:
                d = [[e, None] if e in x else [None, e] for e in sx ^ sy]

        return None if all(map(lambda x: x is None, d)) else d

    return sort_json(dd(x, y))


def is_diff_array(da: list) -> bool:
    if len(da) != 2 or da == [None, None]:
        return False

    if None in da:
        return True

    def all_of_type(xs, t):
        return all(map(lambda x: isinstance(x, t), xs))

    if all_of_type(da, list) or all_of_type(da, dict):
        return False

    return True


def hashablize(x: Iterable) -> Iterable:
    return (a if isinstance(a, Hashable) else str(a) for a in x)


def sort_json(d: Union[dict, list], sort_lists: bool = False):
    if isinstance(d, list):
        return [sort_json(x) for x in (sorted(d) if sort_lists else d)]

    if isinstance(d, dict):
        return OrderedDict(**{k: sort_json(d[k]) for k in sorted(d.keys())})

    return d


def jdump(jd, only_objs=False):
    if only_objs and not isinstance(jd, (list, dict, str)):
        return jd
    return json.dumps(jd, separators=(",", ":"))


def prettify_diff(
    json_diff: Union[dict, list, int, str, None],
    src: str = "",
    tgt: str = "",
    suppress_bs: bool = True,
    value_limit: int = None,
):
    def sup(x):
        if not isinstance(x, str):
            return x
        if value_limit is not None and len(x) > value_limit:
            x = x[:value_limit] + "..."
        return x

    def suppress(x, y):
        x, y = jdump(x, only_objs=True), jdump(y, only_objs=True)
        if None not in (x, y):
            return x, y
        return sup(x), sup(y)

    def pd(jd):
        if isinstance(jd, list):
            if is_diff_array(jd):
                x, y = jd
                if suppress_bs:
                    x, y = suppress(x, y)

                return [{tgt: x}, {src: y}]

            return [pd(x) for x in jd]

        if isinstance(jd, dict):
            return {k: pd(v) for k, v in jd.items()}

        return jd

    return sort_json(pd(json_diff))
