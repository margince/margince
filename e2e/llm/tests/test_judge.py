import json
import os
import sys
import tempfile
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import judge  # noqa: E402
from tests.fakes import FakeProvider  # noqa: E402

YES = '{"verdict":"yes","reason":"it holds"}'
NO = '{"verdict":"no","reason":"it does not"}'


def messages_reply(text):
    return 200, {"content": [{"type": "text", "text": text}], "usage": {"input_tokens": 1, "output_tokens": 1}}


def chat_reply(text):
    return 200, {"choices": [{"message": {"role": "assistant", "content": text}}]}


class ApiJudgeTest(unittest.TestCase):
    def test_api_judge_reaches_a_verdict_with_the_pinned_model_and_no_tools(self):
        with FakeProvider([messages_reply(YES)]) as provider:
            env = {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "api", "ANTHROPIC_API_KEY": "k",
                   "E2E_LLM_JUDGE_BASE_URL": provider.url}
            with unittest.mock.patch.dict(os.environ, env):
                self.assertEqual(judge.verdict("crit", "ans"), (True, "it holds"))
        body = provider.requests[0][2]
        self.assertEqual(body["model"], "claude-haiku-4-5-20251001")
        self.assertNotIn("tools", body)
        self.assertIn("crit", json.dumps(body["messages"][0]["content"]))

    def test_an_unparseable_reply_is_retried_once_with_the_reason(self):
        with FakeProvider([messages_reply("sure!"), messages_reply(NO)]) as provider:
            env = {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "api", "ANTHROPIC_API_KEY": "k",
                   "E2E_LLM_JUDGE_BASE_URL": provider.url}
            with unittest.mock.patch.dict(os.environ, env):
                self.assertEqual(judge.verdict("crit", "ans"), (False, "it does not"))
        self.assertIn("could not be read", json.dumps(provider.requests[1][2]["messages"][0]["content"]))

    def test_openrouter_judge_records_the_canonical_model_not_the_wire_id(self):
        directory = tempfile.mkdtemp()
        with FakeProvider([chat_reply(NO)]) as provider:
            env = {"E2E_LLM_JUDGE": f"record:{directory}", "E2E_LLM_JUDGE_VIA": "openrouter",
                   "OPENAI_COMPATIBLE_API_KEY": "k", "OPENAI_COMPATIBLE_BASE_URL": provider.url}
            with unittest.mock.patch.dict(os.environ, env):
                self.assertEqual(judge.verdict("crit", "ans"), (False, "it does not"))
        self.assertEqual(provider.requests[0][2]["model"], "anthropic/claude-haiku-4.5")
        [recorded] = os.listdir(directory)
        with open(os.path.join(directory, recorded), encoding="utf-8") as handle:
            self.assertEqual(json.load(handle)["model"], "claude-haiku-4-5-20251001")

    def test_a_refused_judge_is_unavailable_not_a_no(self):
        with FakeProvider([(401, {"error": "bad key"})]) as provider:
            env = {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "api", "ANTHROPIC_API_KEY": "k",
                   "E2E_LLM_JUDGE_BASE_URL": provider.url}
            with unittest.mock.patch.dict(os.environ, env):
                with self.assertRaisesRegex(judge.JudgeUnavailable, "401"):
                    judge.verdict("crit", "ans")

    def test_a_200_carrying_an_error_body_is_unavailable_not_a_crash(self):
        with FakeProvider([(200, {"error": {"message": "upstream"}})]) as provider:
            env = {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "openrouter",
                   "OPENAI_COMPATIBLE_API_KEY": "k", "OPENAI_COMPATIBLE_BASE_URL": provider.url}
            with unittest.mock.patch.dict(os.environ, env):
                with self.assertRaises(judge.JudgeUnavailable):
                    judge.verdict("crit", "ans")

    def test_ready_names_the_missing_key(self):
        env = {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "api"}
        with unittest.mock.patch.dict(os.environ, env, clear=True):
            with self.assertRaisesRegex(judge.JudgeUnavailable, "ANTHROPIC_API_KEY"):
                judge.ready()

    def test_ready_refuses_an_unknown_route(self):
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_JUDGE": "live", "E2E_LLM_JUDGE_VIA": "fax"}):
            with self.assertRaisesRegex(judge.JudgeUnavailable, "cli, api, openrouter"):
                judge.ready()

    def test_ready_needs_no_credential_to_replay(self):
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_JUDGE": "replay:/x", "E2E_LLM_JUDGE_VIA": "api"},
                                      clear=True):
            judge.ready()


if __name__ == "__main__":
    unittest.main()
