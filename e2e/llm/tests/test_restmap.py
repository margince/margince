import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import restmap  # noqa: E402
import transcript  # noqa: E402

OPERATIONS = restmap.load()
FRONTED = restmap.routers()


def call(method, path, body=None):
    return {"type": "tool_use", "id": "t1", "name": restmap.HTTP_TOOL,
            "input": {"method": method, "path": path, **({"body": body} if body is not None else {})}}


def run(*blocks):
    return [
        json.dumps({"type": "system", "subtype": "init", "tools": [restmap.HTTP_TOOL]}),
        json.dumps({"type": "assistant", "message": {"content": list(blocks)}}),
    ]


class GradeTest(unittest.TestCase):
    def test_a_route_is_the_verb_its_operation_declares(self):
        self.assertEqual(restmap.grade(OPERATIONS, "GET", "/v1/commitments"), ("review_commitments", {}))

    def test_a_path_parameter_is_named_and_the_prefix_is_optional(self):
        self.assertEqual(restmap.grade(OPERATIONS, "get", "/contacts/abc?limit=2"),
                         ("read_record", {"limit": "2", "id": "abc"}))

    def test_a_record_named_in_the_path_takes_the_mcp_argument_names(self):
        graded = restmap.grade(OPERATIONS, "GET", "/v1/records/company/c1/tags")
        self.assertEqual(graded[1], {"record_type": "company", "record_id": "c1"})

    def test_a_route_that_runs_no_verb_is_not_graded(self):
        self.assertIsNone(restmap.grade(OPERATIONS, "GET", "/v1/no-such-route"))

    def test_a_verb_is_not_credited_to_another_method(self):
        self.assertNotEqual(restmap.grade(OPERATIONS, "DELETE", "/v1/commitments"), ("review_commitments", {}))


class RewriteTest(unittest.TestCase):
    def test_an_http_call_becomes_its_call_with_its_parameters_and_body(self):
        lines = restmap.rewrite(run(call("POST", "/v1/companies/c1/merge", {"target_id": "c2"})), OPERATIONS, FRONTED, ["merge_records"])
        calls = json.loads(lines[1])["message"]["content"]
        router, field, action = FRONTED["merge_records"]
        self.assertEqual(calls[0]["name"], transcript.tool_name(router))
        self.assertEqual(calls[0]["input"], {"id": "c1", "target_id": "c2", **({field: action} if field else {})})

    def test_a_verb_no_router_fronts_keeps_its_own_name(self):
        lines = restmap.rewrite(run(call("GET", "/v1/contacts/c1")), OPERATIONS, FRONTED, [])
        block = json.loads(lines[1])["message"]["content"][0]
        self.assertEqual((block["name"], block["input"]), (transcript.tool_name("read_record"), {"id": "c1"}))

    def test_a_record_named_in_the_body_takes_the_mcp_argument_names_too(self):
        lines = restmap.rewrite(run(call("DELETE", "/v1/tags/t1/apply", {"entity_type": "company", "entity_id": "c1"})),
                                OPERATIONS, FRONTED, [])
        arguments = json.loads(lines[1])["message"]["content"][0]["input"]
        self.assertEqual((arguments["record_type"], arguments["record_id"]), ("company", "c1"))
        self.assertNotIn("entity_type", arguments)

    def test_a_path_parameter_wins_over_a_body_key_of_the_same_name(self):
        lines = restmap.rewrite(run(call("POST", "/v1/companies/c1/merge", {"id": "other"})), OPERATIONS, FRONTED, [])
        self.assertEqual(json.loads(lines[1])["message"]["content"][0]["input"]["id"], "c1")

    def test_a_verb_a_router_fronts_becomes_the_router_call_that_runs_it(self):
        lines = restmap.rewrite(run(call("GET", "/v1/commitments")), OPERATIONS, FRONTED, [])
        block = json.loads(lines[1])["message"]["content"][0]
        self.assertEqual(block["name"], transcript.tool_name("read_attention"))
        self.assertEqual(block["input"], {"view": "commitments"})

    def test_a_line_that_is_not_json_is_left_for_the_checker_to_skip(self):
        lines = restmap.rewrite(['{"type": "assist'] + run(), OPERATIONS, FRONTED, [])
        self.assertEqual(lines[0], '{"type": "assist')

    def test_a_call_that_runs_no_verb_keeps_its_name(self):
        lines = restmap.rewrite(run(call("GET", "/v1/no-such-route")), OPERATIONS, FRONTED, ["x"])
        self.assertEqual(json.loads(lines[1])["message"]["content"][0]["name"], restmap.HTTP_TOOL)

    def test_the_run_is_offered_the_mcp_surface_and_marked_rest(self):
        init = json.loads(restmap.rewrite(run(), OPERATIONS, FRONTED, ["review_commitments", "read_record"])[0])
        self.assertEqual(init["door"], "rest")
        self.assertEqual(init["tools"], [transcript.tool_name("review_commitments"), transcript.tool_name("read_record")])

    def test_a_skill_read_and_text_pass_through(self):
        skill = {"type": "tool_use", "id": "t2", "name": transcript.tool_name("skill_file"), "input": {"path": "INDEX.md"}}
        text = {"type": "text", "text": "hello"}
        content = json.loads(restmap.rewrite(run(text, skill), OPERATIONS, FRONTED, [])[1])["message"]["content"]
        self.assertEqual(content, [text, skill])


class ScoredAsOverMcpTest(unittest.TestCase):
    """What the rewrite is for: the checker reads a REST run as it reads an MCP one."""

    def scored(self, scenario, offered, *blocks):
        import tempfile

        lines = restmap.rewrite(run(*blocks), OPERATIONS, FRONTED, offered)
        fd, path = tempfile.mkstemp(suffix=".jsonl")
        with os.fdopen(fd, "w") as handle:
            handle.write("\n".join(lines) + "\n")
        self.addCleanup(os.remove, path)
        return check.offered_problem(scenario, path), [p for p in check.check(scenario, path) if "never called" in p]

    def test_a_rest_call_satisfies_the_verb_a_scenario_names(self):
        scenario = {"must_call": ["review_commitments"]}
        self.assertEqual(self.scored(scenario, ["review_commitments"], call("GET", "/v1/commitments")), ("", []))

    def test_a_rest_call_satisfies_the_router_a_scenario_names(self):
        router, field, action = FRONTED["remove_tag"]
        scenario = {"must_call": [router], "must_call_with": [f"{router}.{field}={action}"] if field else []}
        problem, missing = self.scored(scenario, [router], call("DELETE", "/v1/tags/t1/apply"))
        self.assertEqual((problem, missing), ("", []))

    def test_a_run_that_made_no_such_call_still_fails_the_scenario(self):
        scenario = {"must_call": ["review_commitments"]}
        _problem, missing = self.scored(scenario, ["review_commitments"], call("GET", "/v1/contacts"))
        self.assertTrue(missing)


if __name__ == "__main__":
    unittest.main()
