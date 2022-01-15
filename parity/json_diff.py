from copy import deepcopy
from typing import List, Union

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
            sx, sy = set(x), set(y)
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

    return dd(x, y)
