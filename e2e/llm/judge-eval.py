#!/usr/bin/env python3
"""Put ONE judge model on trial and write the record the coverage page reads.

WHY THIS EXISTS. The semantic half of every criterion is decided by a model, so
a pass rate on docs/reference/mcp-tool-coverage.md carries that model's accuracy
whether or not anybody prints it. A judge wrong in the quiet direction — passing
an answer the criterion fails — turns a missed defect into a green run, so the
judge's accuracy is a fact the page has to be able to state and a reader has to
be able to reproduce.

THE GROUND TRUTH IS NOT ANOTHER MODEL'S OPINION. e2e/llm/testdata/judge holds
185 verdicts recorded by whichever model is pinned; scoring a judge against them
measures resemblance to that model and hands it a free hundred per cent when it
IS that model. The fixtures under e2e/llm/testdata/<case>/ carry a better label:
a person wrote each answer to be right or wrong on a named criterion, and
scripts/test-e2e-llm-check.sh states which. So this drives that suite with a
LIVE judge and reads its own pass/fail lines — no second copy of the harness,
and no labels this file invents.

    python3 e2e/llm/judge-eval.py claude-haiku-4-5-20251001

It bills tokens (one live judge call per judged criterion) and writes
backend/internal/compose/aicert/records/judge_eval/<model>.json, which
TestTheMCPToolCoverageIsPublished folds into the page.
"""

import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RECORDS = os.path.join(
    ROOT, "backend", "internal", "compose", "aicert", "records", "judge_eval"
)

# A judged fixture is named "<case>/<fixture>" by the suite. The rest of its
# cases — the usage reader, the probe, the refused-credential paths, the regex
# half — are decided by no judge, and scoring over them would answer "what
# fraction of this suite passed while that judge was configured" under a heading
# that says the judge was scored against fixtures.
JUDGED = re.compile(r"^case\d+/")


def main():
    if len(sys.argv) != 2:
        print(__doc__.strip().splitlines()[-3].strip(), file=sys.stderr)
        return 2
    model = sys.argv[1]
    done = subprocess.run(
        [os.path.join(ROOT, "scripts", "test-e2e-llm-check.sh")],
        capture_output=True,
        text=True,
        cwd=ROOT,
        env=dict(os.environ, E2E_LLM_JUDGE="live", E2E_LLM_JUDGE_MODEL=model),
        check=False,
    )
    passed = re.findall(r"^ok: (.+)$", done.stdout, re.M)
    failed = re.findall(r"^FAIL: (.+?)(?: —|$)", done.stdout, re.M)
    if not passed and not failed:
        # A run that scored nothing is not a judge that got everything right.
        print("the suite reported no case at all:\n" + done.stdout[-2000:], file=sys.stderr)
        return 1

    os.makedirs(RECORDS, exist_ok=True)
    record = {
        "model": model,
        "ground_truth": "human-authored fixtures in e2e/llm/testdata/<case>/, "
        "labelled by scripts/test-e2e-llm-check.sh",
        "suite_exit": done.returncode,
        "cases_passed": len(passed),
        "cases_failed": len(failed),
        "judged_fixtures_passed": len([n for n in passed if JUDGED.match(n)]),
        "judged_fixtures_failed": len([n for n in failed if JUDGED.match(n)]),
        "failures": failed,
    }
    path = os.path.join(RECORDS, model + ".json")
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(record, handle, indent=2)
        handle.write("\n")
    print(
        f"{model}: {record['judged_fixtures_passed']} of "
        f"{record['judged_fixtures_passed'] + record['judged_fixtures_failed']} "
        f"judged fixtures -> {path}"
    )
    for failure in failed:
        print("   FAIL", failure)
    return 0


if __name__ == "__main__":
    sys.exit(main())
