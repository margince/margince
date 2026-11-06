import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
import unittest.mock

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ROOT = os.path.dirname(os.path.dirname(HERE))
sys.path.insert(0, HERE)

import check  # noqa: E402
import transcript  # noqa: E402

BUILTIN = [{"name": "cc-plugin-privacy", "path": "builtin", "source": "cc-plugin-privacy@builtin"}]
CLEAN_INIT = {"type": "system", "subtype": "init", "cwd": tempfile.gettempdir(), "plugins": BUILTIN,
              "skills": [], "mcp_servers": [{"name": transcript.SERVER, "status": "connected"}], "tools": []}


def scratch(case):
    """A temporary directory removed when the test ends."""
    path = tempfile.mkdtemp()
    case.addCleanup(shutil.rmtree, path, ignore_errors=True)
    return path


def written(case, *events):
    path = os.path.join(scratch(case), "run.jsonl")
    with open(path, "w", encoding="utf-8") as out:
        for event in events:
            out.write(json.dumps(event) + "\n")
    return path


class UncleanTest(unittest.TestCase):
    """The init line the claude CLI writes says what it loaded besides the lane's server."""

    def test_a_transcript_with_no_init_line_fails(self):
        result = {"type": "result", "is_error": False, "result": "ok", "num_turns": 1}
        self.assertIn("no init line", check.unclean(written(self, result)))

    def test_a_clean_launch_passes(self):
        self.assertEqual(check.unclean(written(self, CLEAN_INIT)), "")

    def test_a_launch_from_inside_the_repository_fails(self):
        self.assertIn("inside the repository", check.unclean(written(self, dict(CLEAN_INIT, cwd=HERE))))

    def test_a_repository_folder_added_to_the_session_fails(self):
        init = dict(CLEAN_INIT, additional_directories=[ROOT])
        self.assertIn("inside the repository", check.unclean(written(self, init)))

    def test_the_operators_memory_fails(self):
        init = dict(CLEAN_INIT, memory_paths={"auto": "/home/operator/.claude/projects/x/memory/"})
        self.assertIn("memory", check.unclean(written(self, init)))

    def test_a_user_plugin_fails_and_a_builtin_one_does_not(self):
        user = {"name": "superpowers", "path": "/home/operator/.claude/plugins/cache/superpowers/7.0.0"}
        problem = check.unclean(written(self, dict(CLEAN_INIT, plugins=BUILTIN + [user])))
        self.assertIn("superpowers", problem)
        self.assertNotIn("cc-plugin-privacy", problem)

    def test_a_loaded_skill_fails(self):
        init = dict(CLEAN_INIT, skills=["superpowers:brainstorming"])
        self.assertIn("skills", check.unclean(written(self, init)))

    def test_an_operator_hook_fails(self):
        hook = {"type": "system", "subtype": "hook_started", "hook_name": "SessionStart:startup"}
        self.assertIn("SessionStart", check.unclean(written(self, hook, CLEAN_INIT)))

    def test_the_lanes_check_stops_a_leaky_run_and_passes_a_clean_one(self):
        leaky = written(self, dict(CLEAN_INIT, cwd=ROOT))
        for path, code in ((leaky, 1), (written(self, CLEAN_INIT), 0)):
            done = subprocess.run([sys.executable, os.path.join(HERE, "check.py"), "--clean", path],
                                  capture_output=True, text=True, check=False)
            self.assertEqual(done.returncode, code, done.stdout + done.stderr)

    def test_a_driver_that_writes_its_own_init_line_passes(self):
        # The bridge and the codex route write the init line themselves, from the lane.
        path = os.path.join(scratch(self), "run.jsonl")
        with transcript.Transcript(path) as out:
            out.init("gpt-5.6-sol", "gpt:cli", "cli-default", "listed-by-lane", ["list_pipelines"])
        self.assertEqual(check.unclean(path), "")


# Models the pinned CLI: what it loads follows from where it starts and how it is told to.
FAKE_CLAUDE = """#!/usr/bin/env python3
import json, os, sys
argv = sys.argv[1:]
sources = argv[argv.index("--setting-sources") + 1] if "--setting-sources" in argv else "user,project,local"
init = {"type": "system", "subtype": "init", "cwd": os.getcwd(), "tools": [],
        "mcp_servers": [{"name": "margince_e2e_llm", "status": "connected"}],
        "plugins": [{"name": "cc-plugin-privacy", "path": "builtin"}],
        "skills": [] if "--disable-slash-commands" in argv else ["deep-research"]}
if "user" in sources:
    init["plugins"].append({"name": "superpowers", "path": os.path.expanduser("~/.claude/plugins/superpowers")})
if os.environ.get("CLAUDE_CODE_DISABLE_AUTO_MEMORY") != "1":
    init["memory_paths"] = {"auto": os.path.expanduser("~/.claude/projects/x/memory/")}
with open(os.environ["FAKE_CLAUDE_SEEN"], "w") as seen:
    json.dump({"argv": argv, "cwd": os.getcwd(), "listing": os.listdir("."),
               "memory_off": os.environ.get("CLAUDE_CODE_DISABLE_AUTO_MEMORY")}, seen)
for event in (init, {"type": "assistant", "message": {"content": [{"type": "text", "text": "ok"}]}},
              {"type": "result", "is_error": False, "result": "ok", "num_turns": 1}):
    print(json.dumps(event))
"""


def lane_function(name):
    """One function of scripts/e2e-llm.sh, as the lane defines it."""
    with open(os.path.join(ROOT, "scripts", "e2e-llm.sh"), encoding="utf-8") as script:
        found = re.search(rf"^{name}\(\) {{\n.*?^}}\n", script.read(), re.S | re.M)
    if found is None:
        raise AssertionError(f"scripts/e2e-llm.sh defines no {name}()")
    return found.group(0)


class RunClaudeTest(unittest.TestCase):
    """run_claude from the lane itself, against a CLI that reports what it was given."""

    def setUp(self):
        self.bin = scratch(self)
        fake = os.path.join(self.bin, "claude")
        with open(fake, "w", encoding="utf-8") as handle:
            handle.write(FAKE_CLAUDE)
        os.chmod(fake, 0o755)
        self.seen_file = os.path.join(self.bin, "seen.json")
        self.out = os.path.join(scratch(self), "run.jsonl")
        self.prompt = os.path.join(self.bin, "prompt.txt")
        with open(self.prompt, "w", encoding="utf-8") as handle:
            handle.write("List the pipelines.")
        self.mcp_config = os.path.join(self.bin, "mcp.json")
        with open(self.mcp_config, "w", encoding="utf-8") as handle:
            handle.write('{"mcpServers": {}}')

    def run_lane(self):
        env = dict(os.environ, PATH=self.bin + os.pathsep + os.environ["PATH"], FAKE_CLAUDE_SEEN=self.seen_file,
                   ROOT=ROOT, MODEL="claude-sonnet-5-5", MCP_CONFIG=self.mcp_config, MCP_SERVER=transcript.SERVER)
        env.pop("CLAUDE_CODE_DISABLE_AUTO_MEMORY", None)
        script = lane_function("run_claude") + f'run_claude "{self.prompt}" "{self.out}"\n'
        done = subprocess.run(["bash", "-c", script], cwd=ROOT, env=env, capture_output=True, text=True, check=False)
        self.assertEqual(done.returncode, 0, done.stderr)
        with open(self.seen_file, encoding="utf-8") as handle:
            return json.load(handle)

    def test_the_lane_starts_claude_clean_from_the_repository_root(self):
        seen = self.run_lane()
        self.assertEqual(check.unclean(self.out), "")
        self.assertFalse(check._inside_repo(seen["cwd"]))
        self.assertEqual(seen["listing"], [])
        self.assertEqual(seen["argv"][seen["argv"].index("--setting-sources") + 1], "")
        self.assertIn("--disable-slash-commands", seen["argv"])
        self.assertEqual(seen["memory_off"], "1")
        self.assertTrue(os.path.isabs(seen["argv"][seen["argv"].index("--mcp-config") + 1]))

    def test_the_empty_directory_is_removed_after_the_run(self):
        seen = self.run_lane()
        self.assertFalse(os.path.exists(seen["cwd"]))

    def test_the_judge_starts_claude_clean_too(self):
        import judge

        env = {"PATH": self.bin + os.pathsep + os.environ["PATH"], "FAKE_CLAUDE_SEEN": self.seen_file,
               "ANTHROPIC_API_KEY": "test-only"}
        with unittest.mock.patch.dict(os.environ, env):
            os.environ.pop("CLAUDE_CODE_DISABLE_AUTO_MEMORY", None)
            judge._cli_asker("claude-haiku-4-5-20251001")("Does the answer name the pipeline?")
        with open(self.seen_file, encoding="utf-8") as handle:
            seen = json.load(handle)
        self.assertFalse(check._inside_repo(seen["cwd"]))
        self.assertEqual(seen["listing"], [])
        self.assertEqual(seen["argv"][seen["argv"].index("--setting-sources") + 1], "")
        self.assertEqual(seen["memory_off"], "1")


if __name__ == "__main__":
    unittest.main()
