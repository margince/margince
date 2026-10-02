import os
import sys
import unittest

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

    def test_openrouter_base_url_defaults_and_can_be_moved(self):
        self.assertEqual(candidates.resolve("mistral", "openrouter").base_url, "https://openrouter.ai/api/v1")
        os.environ["OPENAI_COMPATIBLE_BASE_URL"] = "http://127.0.0.1:1/v1"
        try:
            self.assertEqual(candidates.resolve("mistral", "openrouter").base_url, "http://127.0.0.1:1/v1")
        finally:
            del os.environ["OPENAI_COMPATIBLE_BASE_URL"]


if __name__ == "__main__":
    unittest.main()
