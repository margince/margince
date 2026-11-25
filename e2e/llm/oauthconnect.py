#!/usr/bin/env python3
"""Connect a passport the way an AI client connects one: through OAuth.

A passport minted over REST has no connection, so the server takes every call
on it for a script's, with nobody in a conversation to ask. A call that waits
for its own user then waits in the approval inbox, where a connected client
would be answered a preview and a confirm_code. The lane stands for a person in
a chat, so its passports are connected: it registers a client, plays the
browser's half of the consent with a seat's session, and redeems the code.

    oauthconnect.py <app base URL> <cookie jar written by curl> <client name> <scope>...

Prints the access token. Exits 1 and names the step that failed.
"""

import base64
import hashlib
import http.cookies
import json
import os
import secrets
import sys
import urllib.error
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from casesetup import cookie_header  # noqa: E402

# Never fetched: the code is read from the Location header the server answers.
REDIRECT = "http://127.0.0.1:9/callback"
CONSENT_COOKIE = "crm_oauth_consent"


class ConnectFault(Exception):
    """A step of the handshake did not answer as a client expects."""


class _Answer:
    def __init__(self, status, headers, body):
        self.status, self.headers, self.body = status, headers, body

    def location(self):
        return self.headers.get("Location") or ""


class _KeepRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        return None


_OPEN = urllib.request.build_opener(_KeepRedirects).open


def _send(base, method, path, cookie="", body=None, form=None):
    request = urllib.request.Request(base + path, method=method)
    if cookie:
        request.add_header("Cookie", cookie)
    if body is not None:
        request.data = json.dumps(body).encode()
        request.add_header("Content-Type", "application/json")
    if form is not None:
        request.data = urllib.parse.urlencode(form).encode()
        request.add_header("Content-Type", "application/x-www-form-urlencoded")
    try:
        with _OPEN(request, timeout=60) as response:
            return _Answer(response.status, response.headers, response.read().decode("utf-8", "replace"))
    except urllib.error.HTTPError as answered:
        return _Answer(answered.code, answered.headers, answered.read().decode("utf-8", "replace"))


def _expect(answer, status, step):
    if answer.status != status:
        raise ConnectFault(f"{step} answered HTTP {answer.status}, want {status}: {answer.body[:300]}")


def _set_cookie(answer, name):
    for header in answer.headers.get_all("Set-Cookie") or []:
        jar = http.cookies.SimpleCookie(header)
        if name in jar and jar[name].value:
            return jar[name].value
    return ""


def pkce_pair():
    """(verifier, S256 challenge), RFC 7636."""
    verifier = secrets.token_urlsafe(48)
    digest = hashlib.sha256(verifier.encode()).digest()
    return verifier, base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def connect(base, session_cookie, client_name, scopes):
    """The access token of a passport connected for the seat whose session
    cookie this is, holding `scopes`."""
    base = base.rstrip("/")
    registered = _send(base, "POST", "/oauth/register",
                       body={"client_name": client_name, "redirect_uris": [REDIRECT]})
    _expect(registered, 201, "registering the client")
    client = json.loads(registered.body).get("client_id")
    if not client:
        raise ConnectFault(f"registering the client answered no client_id: {registered.body[:300]}")

    verifier, challenge = pkce_pair()
    request = {"response_type": "code", "client_id": client, "redirect_uri": REDIRECT,
               "scope": " ".join(scopes), "state": secrets.token_urlsafe(8),
               "code_challenge": challenge, "code_challenge_method": "S256"}
    asked = _send(base, "GET", "/oauth/authorize?" + urllib.parse.urlencode(request), session_cookie)
    _expect(asked, 302, "asking for consent")
    screen = urllib.parse.urlsplit(asked.location()).fragment.partition("?")[2]
    nonce = (urllib.parse.parse_qs(screen).get("consent") or [""])[0]
    armed = _set_cookie(asked, CONSENT_COOKIE)
    if not nonce or not armed:
        raise ConnectFault(f"asking for consent led to no consent screen: {asked.location()[:300]}")

    consented = _send(base, "POST", "/oauth/authorize", f"{session_cookie}; {CONSENT_COOKIE}={armed}",
                      form={**request, "consent": nonce, "scopes": " ".join(scopes)})
    _expect(consented, 302, "consenting")
    code = (urllib.parse.parse_qs(urllib.parse.urlsplit(consented.location()).query).get("code") or [""])[0]
    if not code:
        raise ConnectFault(f"consenting answered no code: {consented.location()[:300]}")

    redeemed = _send(base, "POST", "/oauth/token", form={
        "grant_type": "authorization_code", "client_id": client, "redirect_uri": REDIRECT,
        "code_verifier": verifier, "code": code})
    _expect(redeemed, 200, "redeeming the code")
    token = json.loads(redeemed.body).get("access_token")
    if not token:
        raise ConnectFault(f"redeeming the code answered no access_token: {redeemed.body[:300]}")
    return token


def main(argv):
    if len(argv) < 5:
        print(__doc__, file=sys.stderr)
        return 1
    base, jar, client_name, scopes = argv[1], argv[2], argv[3], argv[4:]
    try:
        with open(jar, encoding="utf-8") as handle:
            cookie = cookie_header(handle.read())
        if not cookie:
            raise ConnectFault(f"the cookie jar {jar} holds no session")
        print(connect(base, cookie, client_name, scopes))
    except (ConnectFault, OSError, ValueError) as fault:
        print(f"connecting {client_name} failed: {fault}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
