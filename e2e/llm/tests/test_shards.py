"""The weekly run's shards cover the folder exactly, and a short return is refused."""

import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import shards  # noqa: E402


class ShardsTest(unittest.TestCase):
    def test_every_scenario_is_in_exactly_one_shard(self):
        planned = shards.plan(8)["include"]
        dealt = [int(n) for shard in planned for n in shard["cases"].split()]
        self.assertEqual(sorted(dealt), [number for number, _ in shards.scenarios()])
        self.assertEqual(len(planned), 8)

    def test_more_shards_than_cases_gives_one_case_each(self):
        directory = tempfile.mkdtemp()
        for number in (3, 1):
            with open(os.path.join(directory, f"case{number}-x.yaml"), "w", encoding="utf-8") as out:
                out.write(f"name: case{number}_x\n")
        self.assertEqual(shards.plan(5, directory)["include"],
                         [{"shard": 1, "cases": "1"}, {"shard": 2, "cases": "3"}])

    def test_a_missing_verdict_is_named(self):
        verdicts = tempfile.mkdtemp()
        names = [name for _, name in shards.scenarios()]
        for name in names[1:]:
            open(os.path.join(verdicts, name + ".json"), "w", encoding="utf-8").close()
        self.assertEqual(shards.missing(verdicts), names[:1])


if __name__ == "__main__":
    unittest.main()
