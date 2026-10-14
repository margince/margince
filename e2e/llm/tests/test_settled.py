import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402


class SettledTest(unittest.TestCase):
    def test_enough_passes_settle_a_pass(self):
        self.assertTrue(check.settled(passed=2, made=2, runs=3, pass_at=2))

    def test_too_many_failures_settle_a_fail(self):
        self.assertTrue(check.settled(passed=0, made=2, runs=3, pass_at=2))

    def test_a_split_is_not_settled(self):
        self.assertFalse(check.settled(passed=1, made=2, runs=3, pass_at=2))

    def test_one_run_settles_nothing_at_two_of_three(self):
        self.assertFalse(check.settled(passed=1, made=1, runs=3, pass_at=2))
        self.assertFalse(check.settled(passed=0, made=1, runs=3, pass_at=2))

    def test_the_last_run_always_settles(self):
        self.assertTrue(check.settled(passed=1, made=3, runs=3, pass_at=2))

    def test_a_pass_at_equal_to_runs_fails_on_the_first_failure(self):
        self.assertTrue(check.settled(passed=0, made=1, runs=3, pass_at=3))
        self.assertFalse(check.settled(passed=1, made=1, runs=3, pass_at=3))


if __name__ == "__main__":
    unittest.main()
