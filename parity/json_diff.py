from copy import deepcopy
import json
from typing import List, Union, Tuple
from collections import OrderedDict


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


def sort_json(d: Union[dict, list], sort_lists: bool = False):
    if isinstance(d, list):
        return [sort_json(x) for x in (sorted(d) if sort_lists else d)]

    if isinstance(d, dict):
        return OrderedDict(**{k: sort_json(d[k]) for k in sorted(d.keys())})

    return d


def flatten_diff(
    json_diff: Union[dict, list, int, str, None],
    blank_diff_path=True,
    src: str = None,
    tgt: str = None,
    spacer: str = None,
) -> List[str]:
    if src and (not tgt):
        tgt = " " * len(src)
    if tgt and (not src):
        src = " " * len(tgt)

    fw_src, fw_tgt = src, tgt
    if src:
        padlen = len(src) - len(tgt)
        if padlen > 0:
            tgt += " " * padlen
        else:
            src += " " * -padlen
        tgt += "--->"
        src += "--->"
    else:
        src = tgt = ""

    def dump(stack, jd, src_or_tgt):
        is_src = src_or_tgt == "src"
        path = ".".join(map(str, stack))
        if blank_diff_path and is_src:
            path = " " * len(path)

        return (
            (src if is_src else tgt)
            + path
            + ":"
            + json.dumps(jd, separators=(",", ":"))
        )

    def fd(jd, stack=[]) -> list:
        if isinstance(jd, list):
            if not stack or not is_diff_array(jd):
                lines = []
                for i, x in enumerate(jd):
                    lines.extend(fd(x, stack + [i]))
                return lines

            # WLOG jd is a diff array (except at the top level)
            return [dump(stack, jd[0], "tgt"), dump(stack, jd[1], "src")]

        if isinstance(jd, dict):
            lines = []
            for k, v in jd.items():
                lines.extend(fd(v, stack + [k]))
            return lines

        # jd is a simple type:
        return [dump(stack, jd, False)]

    def analysis(target, source):
        if source.endswith("null"):
            return f"{fw_src} missing attribute present in {fw_tgt}"
        if target.endswith("null"):
            return f"{fw_src} has attribute missing from {fw_tgt}  "
        return f"{fw_src} and {fw_tgt} disagree on an attribute"

    def insert_spacer(pairs):
        res = []
        n = len(pairs)
        for i in range(n // 2):
            target, source = pairs[2 * i], pairs[2 * i + 1]
            res.extend([spacer.format(analysis(target, source)), target, source])
        if 2 * i + 2 < n:
            res.append(pairs[2 * i + 2])
        return res

    pairs = fd(json_diff)
    return insert_spacer(pairs) if spacer else pairs


def report_diff(
    json_diff: Union[dict, list, int, str, None],
    blank_diff_path=True,
    src: str = None,
    tgt: str = None,
    spacer: str = None,
) -> Tuple[str, int]:
    flattened = flatten_diff(
        json_diff, blank_diff_path=blank_diff_path, src=src, tgt=tgt, spacer=spacer
    )
    return "\n".join(flattened), len(flattened) / 2
