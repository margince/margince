#!/usr/bin/env python3
"""Grade one answer against several criteria with this lane's judge.

The agent_loop lane in backend/internal/compose/agentloope2e is written in Go
and must not grow a second judge. This is its door to judge.py: one JSON object
on stdin, one JSON list on stdout.

    stdin   {"criteria": ["Does the answer ...?", ...], "answer": "..."}
    stdout  [{"criterion": "...", "met": true, "reason": "..."}, ...]

`--model` prints the model the judge is pinned to and asks nothing. `--ready`
checks that a verdict could be reached, so a lane can stop before it pays for
the work the judge would grade.

A judge that cannot be reached exits 3 with the reason on stderr and prints no
verdict. A caller that read silence as a pass would be the defect that
judge.JudgeUnavailable exists to prevent. Any other failure exits 1.
"""

import json
import sys

import judge

EXIT_UNAVAILABLE = 3
EXIT_BAD_INPUT = 2


def grade(request):
    """One verdict per criterion, in the order the request lists them."""
    criteria, answer = request.get("criteria"), request.get("answer")
    if (
        not isinstance(criteria, list)
        or not criteria
        or not all(isinstance(item, str) and item.strip() for item in criteria)
        or not isinstance(answer, str)
    ):
        raise ValueError('expected {"criteria": [non-empty strings], "answer": string}')
    judge.ready()
    out = []
    for criterion in criteria:
        met, reason = judge.verdict(criterion, answer)
        out.append({"criterion": criterion, "met": met, "reason": reason})
    return out


def main(argv, stdin, stdout, stderr):
    if argv == ["--model"]:
        stdout.write(judge.pinned_model() + "\n")
        return 0
    try:
        if argv == ["--ready"]:
            judge.ready()
            return 0
        request = json.load(stdin)
        verdicts = grade(request)
    except judge.JudgeUnavailable as err:
        stderr.write(f"judge unavailable: {err}\n")
        return EXIT_UNAVAILABLE
    except (ValueError, AttributeError) as err:
        stderr.write(f"bad request: {err}\n")
        return EXIT_BAD_INPUT
    json.dump(verdicts, stdout, ensure_ascii=False)
    stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:], sys.stdin, sys.stdout, sys.stderr))
