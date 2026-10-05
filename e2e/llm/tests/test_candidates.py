import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import candidates  # noqa: E402


class ResolveTest(unittest.TestCase):
    def test_default_api_route_files_under_canonical_folder(self):
        route = candidates.resolve("gpt", "api")
        self.assertEqual(
            (route.model, route.folder, route.wire, route.key_env),
            ("gpt-5.6-sol", "gpt-5.6-sol", "responses", "OPENAI_API_KEY"),
        )

    def test_openrouter_shares_the_canonical_folder(self):
        route = candidates.resolve("claude", "openrouter")
        self.assertEqual((route.model, route.folder), ("anthropic/claude-sonnet-5.5", "claude-sonnet-5-5"))

    def test_cli_route_is_filed_apart(self):
        self.assertEqual(candidates.resolve("claude", "cli").folder, "claude-sonnet-5-5@claude-cli")
        self.assertEqual(candidates.resolve("gpt", "cli").folder, "gpt-5.6-sol@codex-cli")

    def test_mistral_has_no_cli(self):
        with self.assertRaisesRegex(candidates.RouteError, "mistral has no cli route"):
            candidates.resolve("mistral", "cli")

    def test_unknown_candidate_names_the_known_ones(self):
        with self.assertRaisesRegex(candidates.RouteError, "claude, gpt, mistral"):
            candidates.resolve("gemini", "api")

    def test_model_override_needs_a_folder_unless_it_is_the_default(self):
        route = candidates.resolve("claude", "cli", model="claude-opus-5", folder="claude-opus-5")
        self.assertEqual((route.model, route.folder), ("claude-opus-5", "claude-opus-5@claude-cli"))
        with self.assertRaisesRegex(candidates.RouteError, "E2E_LLM_FOLDER"):
            candidates.resolve("claude", "api", model="claude-opus-5")

    def test_slash_folder_refused(self):
        with self.assertRaisesRegex(candidates.RouteError, "E2E_LLM_FOLDER"):
            candidates.resolve("gpt", "openrouter", model="openai/gpt-5.5", folder="openai/gpt-5.5")

    def test_an_effort_experiment_needs_its_own_folder(self):
        with self.assertRaisesRegex(candidates.RouteError, "E2E_LLM_FOLDER"):
            candidates.resolve("mistral", "openrouter", effort="reasoning high")
        route = candidates.resolve("mistral", "openrouter", effort="reasoning high",
                                   folder="mistral-medium-3-5-reasoning-high")
        self.assertEqual((route.effort, route.folder),
                         ("reasoning high", "mistral-medium-3-5-reasoning-high"))

    def test_the_default_effort_named_out_loud_is_not_an_experiment(self):
        self.assertEqual(candidates.resolve("gpt", "api", effort="reasoning medium").folder, "gpt-5.6-sol")

    def test_an_effort_the_wire_cannot_express_is_refused(self):
        with self.assertRaisesRegex(candidates.RouteError, "reasoning low|medium|high"):
            candidates.resolve("mistral", "openrouter", effort="think hard", folder="x")

    def test_openrouter_base_url_defaults_and_can_be_moved(self):
        with unittest.mock.patch.dict(os.environ, {"OPENAI_COMPATIBLE_BASE_URL": ""}):
            self.assertEqual(candidates.resolve("mistral", "openrouter").base_url, "https://openrouter.ai/api/v1")
        with unittest.mock.patch.dict(os.environ, {"OPENAI_COMPATIBLE_BASE_URL": "http://127.0.0.1:1/v1"}):
            self.assertEqual(candidates.resolve("mistral", "openrouter").base_url, "http://127.0.0.1:1/v1")

    def test_check_refuses_a_short_candidate_line_cleanly(self):
        import subprocess
        here = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        done = subprocess.run([sys.executable, os.path.join(here, "check.py"), "--candidate", "claude"],
                              capture_output=True, text=True, check=False)
        self.assertEqual(done.returncode, 1)
        self.assertNotIn("Traceback", done.stderr)


if __name__ == "__main__":
    unittest.main()
