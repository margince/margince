import os
import re
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import scenariolint  # noqa: E402

SCENARIOS = os.path.join(HERE, "scenarios")


def pin(scenario, key, marker):
    """The one committed pin of a scenario that carries the marker text."""
    found = [p for p in check.parse_scenario(os.path.join(SCENARIOS, scenario)).get(key, []) if marker in p]
    if len(found) != 1:
        raise AssertionError(f"{scenario} has {len(found)} {key} pins carrying {marker!r}")
    return found[0]


class ContractedNegationTest(unittest.TestCase):
    """A negation an answer contracts reads the same as the one it spells out."""

    def test_a_positive_pin_holds_the_contracted_answer_as_well(self):
        for scenario, marker, spelled, contracted in (
            ("case3-spreadsheet.yaml", "imported", "Not imported: the row with no name.",
             "It wasn't imported, the row with no name."),
            ("case3-spreadsheet.yaml", "imported", "The last row could not be used.", "The last row couldn't be used."),
            ("case40-sort-the-queue.yaml", "going anywhere", "Bruno is not going anywhere, so Bruno is closed.",
             "Bruno isn't going anywhere, so Bruno is closed."),
            ("case40-sort-the-queue.yaml", "recorded", "The title is not recorded.", "The title hasn't been recorded."),
            ("case56-not-the-same.yaml", "a duplicate", "They are not the same contact.", "They aren't the same contact."),
            ("case64-catch-me-up-and-the-brief.yaml", "brief", "The brief has not been generated.",
             "The brief hasn't been generated."),
        ):
            pattern = pin(scenario, "must_mention", marker)
            with self.subTest(scenario=scenario, marker=marker):
                self.assertRegex(spelled, pattern)
                self.assertRegex(contracted, pattern)

    def test_an_exclusion_lets_no_contracted_denial_through(self):
        done = pin("case10-finish-the-import.yaml", "must_mention", "committed|imported|created")
        self.assertRegex("The four companies are imported.", done)
        self.assertIsNone(re.search(done, "I haven't imported them yet."))
        self.assertIsNone(re.search(done, "I have not imported them yet."))


def lint_problems(pattern):
    handle = tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False, encoding="utf-8")
    with handle as out:
        out.write('name: case999_lint\ncriteria: [1]\nwrites: false\nprompt: |\n  Hi.\n'
                  f'must_mention:\n  - "{pattern}"\nruns: 3\npass_at: 2\n')
    return [p for p in scenariolint.lint(handle.name) if "n't form" in p]


class NegationLintTest(unittest.TestCase):
    def test_a_pin_with_a_bare_not_fails_lint(self):
        self.assertEqual(len(lint_problems(r"(?i)not (?:sent|delivered)")), 1)

    def test_a_pin_that_also_reads_the_contraction_lints_clean(self):
        for pattern in (r"(?i)(?:not|n['’]t) (?:sent|delivered)", r"(?i)(?:can'?t|could not) send", r"(?i)nothing was sent"):
            with self.subTest(pattern=pattern):
                self.assertEqual(lint_problems(pattern), [])


if __name__ == "__main__":
    unittest.main()
