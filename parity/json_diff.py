from copy import deepcopy
from curses.ascii import DEL
import json
from typing import Callable, List, Union, Tuple
from collections import OrderedDict


L, R = "left", "right"

REPORT, SUMMARY = "report", "summary"

CONFLICT, DELETION, ADDITION = "conflict", "deletion", "addition"


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

                # return [f"[{tgt:^10}] --> {x}", f"[{src:^10}] --> {y}"]
                return [{tgt: x}, {src: y}]

            return [pd(x) for x in jd]

        if isinstance(jd, dict):
            return {k: pd(v) for k, v in jd.items()}

        return jd

    return sort_json(pd(json_diff))


def flatten_diff(
    json_diff: Union[dict, list, int, str, None],
    output: Union[REPORT, SUMMARY] = REPORT,
    blank_diff_path=True,
    src: str = None,
    tgt: str = None,
    spacer: str = None,
    extra_lines: int = 0,
    must_be_even: bool = False,
) -> Tuple[List[str], int]:
    """
    json_diff: output of deep_diff()
    blank_diff_path: when True, replace the path string for the source with blanks
    output: when REPORT, show the diffs 2 lines a time, when SUMMARY, just show the path and diff type
    src: tag for the source JSON file (e.g. "ALGOD")
    tgt: tag for the target JSON file (e.g. "INDEXER")
    spacer: formattable heading string to begin each diff pair with (e.g. "----------{}----------")
    extra_lines: the number of empty lines to put at the end of each diff pair
    must_be_even: assert that the number of diff lines is even (recommend set True for standard reports)
    """
    assert output in (REPORT, SUMMARY), f"encountered unknown output type [{output}]"

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

    SUMMARY_SEP = "###$$$###"

    def dump(stack, jd, src_or_tgt):
        is_src = src_or_tgt == "src"
        path = ".".join(map(str, stack))
        if blank_diff_path and is_src:
            path = " " * len(path)

        return (src if is_src else tgt) + path + ":" + jdump(jd)

    def analysis(target, source):
        if source.endswith("null"):
            return f"{fw_src} missing attribute present in {fw_tgt}"
        if target.endswith("null"):
            return f"{fw_src} has attribute missing from {fw_tgt}  "
        return f"{fw_src} and {fw_tgt} disagree on an attribute"

    def insert_spacers(pairs):
        res = []
        n = len(pairs)
        i = -1
        for i in range(n // 2):
            target, source = pairs[2 * i], pairs[2 * i + 1]
            group = [spacer.format(analysis(target, source))] if spacer else []
            group += [target, source]
            for _ in range(extra_lines):
                group.append("")
            res.extend(group)
        if must_be_even:
            assert 2 * i + 2 == n, "oops, we have an odd number of lines!!!"
        elif 2 * i + 2 < n:
            res.append(pairs[2 * i + 2])

        return res

    def report_grouper(stack: List[str], diff_array: Tuple["tgt", "src"]) -> List[str]:
        """
        lambda that plugs into fd() for diff reports.
        type-hint for diff_array is a complete lie, but gets the point across
        """
        return [dump(stack, diff_array[0], "tgt"), dump(stack, diff_array[1], "src")]

    def summary_grouper(
        stack: List[str], diff_array: Tuple["tgt", "src"]
    ) -> List[Tuple[str, str]]:
        """
        lambda that plugs into fd() for summary reports.
        type-hint for diff_array is a complete lie, but gets the point across
        Returns: (path-summary, diff-type)
        """
        assert not (
            diff_array[0] is None and diff_array[1] is None
        ), "When both values are None, this shouldn't be treated as a diff-array"
        diff_type = CONFLICT
        if diff_array[0] is None:
            diff_type = DELETION
        elif diff_array[1] is None:
            diff_type = ADDITION

        pref = ".".join(map(str, stack))
        if diff_type == CONFLICT:
            stub = " " * len(pref)
            tgt_line = f"{pref}:{tgt}{jdump(diff_array[0])}"
            src_line = f"{stub}:{src}{jdump(diff_array[1])}"
            summary = tgt_line + "\n" + src_line
        else:
            suffidx = int(diff_type == DELETION)
            mid = src if diff_type == DELETION else tgt
            summary = f"{pref}:{mid}{jdump(diff_array[suffidx])}"
        return [diff_type + SUMMARY_SEP + summary]

    def fd(jd, grouper: Callable, stack=[]) -> list:
        if isinstance(jd, list):
            if not stack or not is_diff_array(jd):
                lines = []
                for i, x in enumerate(jd):
                    lines.extend(fd(x, grouper, stack + [i]))
                return lines

            # WLOG jd is a diff array (except at the top level)
            return grouper(stack, jd)

        if isinstance(jd, dict):
            lines = []
            for k, v in jd.items():
                lines.extend(fd(v, grouper, stack + [k]))
            return lines

        # jd is a simple type:
        return [dump(stack, jd, False)]

    if output == REPORT:
        pairs = fd(json_diff, report_grouper)
        return insert_spacers(pairs), len(pairs)

    assert (
        output == SUMMARY
    ), "should have had a better check at the top of the function"
    summaries = fd(json_diff, summary_grouper)
    summaries = [line.split(SUMMARY_SEP) for line in summaries]
    return summaries, len(summaries)


def report_diff(
    json_diff: Union[dict, list, int, str, None],
    blank_diff_path=True,
    src: str = None,
    tgt: str = None,
    spacer: str = None,
    extra_lines: int = 0,
    must_be_even: bool = False,
) -> Tuple[str, int]:
    flattened, num_diffs = flatten_diff(
        json_diff,
        blank_diff_path=blank_diff_path,
        src=src,
        tgt=tgt,
        spacer=spacer,
        extra_lines=extra_lines,
        must_be_even=must_be_even,
    )
    return "\n".join(flattened), num_diffs


def diff_summary(
    json_diff: Union[dict, list, int, str, None],
    src: str = None,
    tgt: str = None,
    spacer: str = None,
) -> Tuple[str, int]:
    diffs, diff_count = flatten_diff(json_diff, output=SUMMARY, src=src, tgt=tgt)
    conflicts = [line for diff_type, line in diffs if diff_type == CONFLICT]
    additions = [line for diff_type, line in diffs if diff_type == ADDITION]
    deletions = [line for diff_type, line in diffs if diff_type == DELETION]
    total_mods = len(conflicts) + len(additions) + len(deletions)
    assert (
        diff_count == total_mods
    ), f"WOOPS - inconsistent diff count: {total_mods} != {diff_count}"

    def heading(diff_type):
        insert = f"{len(conflicts)} Conflicting Attributes"
        if diff_type == DELETION:
            insert = f"{len(deletions)} Deleted Attributes"
        elif diff_type == ADDITION:
            insert = f"{len(additions)} New Attributes"
        return spacer.format(insert) if spacer else insert

    lines = []
    lines.append(heading(ADDITION))
    lines.extend(additions)
    lines.append(heading(DELETION))
    lines.extend(deletions)
    lines.append(heading(CONFLICT))
    lines.extend(conflicts)

    return "\n".join(lines), diff_count
