"""oauthconnect against an in-process authorization server that checks what
the product checks: the session on both consent steps, the consent cookie and
nonce as a pair, and the PKCE verifier against the challenge."""

import base64
import contextlib
import hashlib
import io
import json
import os
import sys
import tempfile
import threading
import unittest
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import oauthconnect  # noqa: E402

SESSION = "crm_session=session-value"
JAR = "# Netscape HTTP Cookie File\n#HttpOnly_localhost\tFALSE\t/\tTRUE\t0\tcrm_session\tsession-value\n"


class FakeAuthorizationServer:
    """Answers the four steps; `refuse` names one to answer with an error."""

    def __init__(self, refuse=""):
        self.challenge, self.granted, self.client_names = "", [], []
        server = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def answer(self, status, body=b"", headers=()):
                self.send_response(status)
                for key, value in headers:
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def body(self):
                return self.rfile.read(int(self.headers.get("Content-Length") or 0)).decode()

            def do_GET(self):
                path, _, query = self.path.partition("?")
                asked = urllib.parse.parse_qs(query)
                if path != "/oauth/authorize" or refuse == "authorize" or SESSION not in (self.headers.get("Cookie") or ""):
                    return self.answer(401, b'{"detail":"sign in"}')
                server.challenge = asked["code_challenge"][0]
                self.answer(302, headers=[
                    ("Location", "http://app/#/oauth-consent?client=x&consent=nonce-1"),
                    ("Set-Cookie", "crm_oauth_consent=nonce-1; Path=/oauth/authorize; HttpOnly; Secure; SameSite=Strict")])

            def do_POST(self):
                raw = self.body()
                if self.path == "/oauth/register":
                    server.client_names.append(json.loads(raw)["client_name"])
                    if refuse == "register":
                        return self.answer(400, b'{"error":"invalid_client_metadata"}')
                    return self.answer(201, json.dumps({"client_id": "client-1"}).encode())
                form = {key: values[0] for key, values in urllib.parse.parse_qs(raw).items()}
                if self.path == "/oauth/authorize":
                    cookie = self.headers.get("Cookie") or ""
                    if refuse == "consent" or SESSION not in cookie or "crm_oauth_consent=nonce-1" not in cookie \
                            or form.get("consent") != "nonce-1":
                        return self.answer(302, headers=[("Location", "http://app/#/oauth-consent?error=invalid")])
                    server.granted = form["scopes"].split()
                    return self.answer(302, headers=[("Location", oauthconnect.REDIRECT + "?code=code-1&state=s")])
                if self.path == "/oauth/token":
                    digest = hashlib.sha256(form.get("code_verifier", "").encode()).digest()
                    if refuse == "token" or form.get("code") != "code-1" \
                            or base64.urlsafe_b64encode(digest).rstrip(b"=").decode() != server.challenge:
                        return self.answer(400, b'{"error":"invalid_grant"}')
                    return self.answer(200, json.dumps({"access_token": "mcp_connected"}).encode())
                self.answer(404)

        self._httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self._httpd.server_port}"

    def __enter__(self):
        threading.Thread(target=self._httpd.serve_forever, args=(0.01,), daemon=True).start()
        return self

    def __exit__(self, *_exc):
        self._httpd.shutdown()
        self._httpd.server_close()


class ConnectTest(unittest.TestCase):
    def test_a_consented_connection_answers_its_token_with_the_scopes_asked(self):
        with FakeAuthorizationServer() as server:
            token = oauthconnect.connect(server.url + "/", SESSION, "e2e-llm-send", ["read", "write", "send"])
        self.assertEqual(token, "mcp_connected")
        self.assertEqual(server.granted, ["read", "write", "send"])
        self.assertEqual(server.client_names, ["e2e-llm-send"])

    def test_each_refused_step_is_named(self):
        for step, named in (("register", "registering the client"), ("authorize", "asking for consent"),
                            ("consent", "consenting"), ("token", "redeeming the code")):
            with self.subTest(step=step), FakeAuthorizationServer(refuse=step) as server:
                with self.assertRaises(oauthconnect.ConnectFault) as fault:
                    oauthconnect.connect(server.url, SESSION, "e2e-llm", ["read"])
                self.assertIn(named, str(fault.exception))

    def test_a_session_the_server_does_not_know_never_reaches_consent(self):
        with FakeAuthorizationServer() as server, self.assertRaises(oauthconnect.ConnectFault) as fault:
            oauthconnect.connect(server.url, "crm_session=stale", "e2e-llm", ["read"])
        self.assertIn("HTTP 401", str(fault.exception))

    def test_the_verifier_is_the_challenges_preimage(self):
        verifier, challenge = oauthconnect.pkce_pair()
        digest = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
        self.assertEqual(challenge, digest)
        self.assertGreaterEqual(len(verifier), 43)


class MainTest(unittest.TestCase):
    def run_main(self, argv):
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = oauthconnect.main(argv)
        return code, out.getvalue(), err.getvalue()

    def test_the_token_is_printed_from_a_curl_jar(self):
        with tempfile.NamedTemporaryFile("w", suffix=".cookies", delete=False) as jar:
            jar.write(JAR)
        self.addCleanup(os.unlink, jar.name)
        with FakeAuthorizationServer() as server:
            code, out, _ = self.run_main(["oauthconnect.py", server.url, jar.name, "e2e-llm", "read", "write"])
        self.assertEqual((code, out), (0, "mcp_connected\n"))

    def test_a_jar_with_no_session_fails_before_any_request(self):
        with tempfile.NamedTemporaryFile("w", suffix=".cookies", delete=False) as jar:
            jar.write("# Netscape HTTP Cookie File\n")
        self.addCleanup(os.unlink, jar.name)
        code, out, err = self.run_main(["oauthconnect.py", "http://127.0.0.1:9", jar.name, "e2e-llm", "read"])
        self.assertEqual((code, out), (1, ""))
        self.assertIn("holds no session", err)

    def test_too_few_arguments_print_the_usage(self):
        code, _, err = self.run_main(["oauthconnect.py", "http://127.0.0.1:9"])
        self.assertEqual(code, 1)
        self.assertIn("oauthconnect.py <app base URL>", err)


if __name__ == "__main__":
    unittest.main()
