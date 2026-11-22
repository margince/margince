import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import scenariolint  # noqa: E402

SCENARIO = """name: case999_lint
criteria: [1]
writes: false
prompt: |
  What changed?
judge:
  - "%s"
runs: 3
pass_at: 2
"""


def problems(criterion):
    with tempfile.TemporaryDirectory() as scratch:
        path = os.path.join(scratch, "case999-lint.yaml")
        with open(path, "w", encoding="utf-8") as out:
            out.write(SCENARIO % criterion)
        return [p for p in scenariolint.lint(path) if "defers to a tool's answer" in p]


class JudgeCriterionLintTest(unittest.TestCase):
    """The judge reads the final answer alone, so a criterion names its facts."""

    def test_a_criterion_that_defers_to_the_tool_fails_lint(self):
        for criterion in (
            "Criterion 1. The causes add up to the difference, as the tool reports them. Answer no otherwise.",
            "Criterion 1. It does not list a deal the tool did not flag. Answer no otherwise.",
            "Criterion 1. It quotes the figure from the standard reporting reading it made. Answer no otherwise.",
            "Criterion 1. Every figure matches the tool result. Answer no otherwise.",
            "Criterion 1. It names only deals that the tool flagged. Answer no otherwise.",
            "Criterion 1. Every figure reported by the tool is quoted. Answer no otherwise.",
            "Criterion 1. It quotes the tool's figures. Answer no otherwise.",
            "Criterion 1. It names the deals the tools flagged. Answer no otherwise.",
            "Criterion 1. It gives the result of the read. Answer no otherwise.",
        ):
            with self.subTest(criterion=criterion):
                self.assertEqual(len(problems(criterion)), 1)

    def test_a_criterion_that_names_its_facts_lints_clean(self):
        for criterion in (
            "Criterion 1. The answer gives EUR 179,000 and EUR 164,500, and the EUR 5,000 repricing and the "
            "EUR 9,500 slip. Answer no if it gives other figures.",
            "Criterion 2. The answer names Körber Sensorik Rollout and lists no other deal as at risk. Answer no otherwise.",
            "Criterion 2. None could go: the workspace has no connected mailbox, and the tools refused. Answer no otherwise.",
        ):
            with self.subTest(criterion=criterion):
                self.assertEqual(problems(criterion), [])


if __name__ == "__main__":
    unittest.main()
