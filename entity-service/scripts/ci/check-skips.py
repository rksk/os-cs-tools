#!/usr/bin/env python3
# Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
"""Fails CI when a test skipped that is not on the allowlist.

Integration tests skip themselves when their database variable is unset or the
database lacks what they need. In CI that is how a test quietly stops running,
so every skip must be a deliberate, listed one.

Also prints the output of every failed test, since the raw JSON stream is not
readable in a CI log.

Usage: check-skips.py <go-test-json-file> <allowlist-file>

The allowlist holds one regular expression per line, matched with fullmatch
against "<package-suffix>/<TestName>[/<subtest>]". A line starting with
"msg: " is instead a regular expression matched (re.search) against the skip
message the test printed, for a whole class of skips. Blank lines and lines
starting with # are ignored. An allowlist line that matched nothing is
reported as a warning (it has gone stale), not a failure.
"""
import json
import re
import sys


def main(results_path, allowlist_path):
    patterns = []  # (text, kind, compiled)
    with open(allowlist_path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if line.startswith("msg: "):
                patterns.append((line, "msg", re.compile(line[5:])))
            else:
                patterns.append((line, "name", re.compile(line)))

    skipped, ran = [], 0
    output, failed = {}, []
    with open(results_path) as f:
        for raw in f:
            try:
                ev = json.loads(raw)
            except ValueError:
                continue
            if not ev.get("Test"):
                # Package-level events: a package can fail with no failing test
                # (setup error, panic, build failure).
                key = (ev["Package"], "")
                if ev["Action"] == "output":
                    output.setdefault(key, []).append(ev["Output"])
                elif ev["Action"] == "fail":
                    failed.append(key)
                continue
            key = (ev["Package"], ev["Test"])
            if ev["Action"] == "output":
                output.setdefault(key, []).append(ev["Output"])
            if ev["Action"] in ("pass", "fail"):
                ran += 1
            if ev["Action"] == "fail":
                failed.append(key)
            elif ev["Action"] == "skip":
                pkg = ev["Package"].split("/entity-service/", 1)[-1]
                skipped.append((f"{pkg}/{ev['Test']}", key))

    # `go test -json` output is unreadable in a CI log; show what failed.
    for pkg, test in failed:
        print(f"--- FAIL {pkg.split('/entity-service/', 1)[-1]}/{test}".rstrip("/"))
        for line in output.get((pkg, test), [])[:40]:
            print("    " + line.rstrip())

    used, unexpected = set(), []
    for name, key in skipped:
        message = " ".join(
            m.group(1)
            for m in (re.search(r"\.go:\d+: (.*)", o) for o in output.get(key, []))
            if m
        )
        for text, kind, rx in patterns:
            hit = rx.fullmatch(name) if kind == "name" else rx.search(message)
            if hit:
                used.add(text)
                break
        else:
            unexpected.append(f"{name}  [{message[:100]}]")

    print(f"tests run: {ran}, skipped: {len(skipped)}, unexpected skips: {len(unexpected)}")
    for text, _, _ in patterns:
        if text not in used:
            print(f"warning: allowlist entry matched no skip (stale?): {text}")
    if unexpected:
        print("\nThese tests skipped but are not on scripts/ci/skip-allowlist.txt.")
        print("A skip in CI usually means a test lost its database. Fix the cause, or")
        print("add the test to the allowlist with a reason if the skip is intended.\n")
        for name in sorted(unexpected):
            print(f"  {name}")
        return 1
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    sys.exit(main(sys.argv[1], sys.argv[2]))
