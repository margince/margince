"""The named steps a scenario's `setup:` runs before each of its runs.

A step is code in this file, never shell from a scenario: a YAML value only
names one. The lane runs it after the world is reset and marks the world dirty
first, so the snapshot restore before the next run undoes whatever it wrote.
That restore is the teardown; a step writes nothing that outlives its case.

A step writes through the REST API as the admin, the human the lane's
passports act for, with the session the lane signed in with.

Usage:
  casesetup.py --known <step>                  exit 0 when the step exists
  casesetup.py <step> <app base URL> <cookie jar written by curl>
"""

import json
import sys
import urllib.error
import urllib.request
import uuid

ADMIN_EMAIL = "admin@demo.test"


class SetupFault(Exception):
    """The step could not leave the world it names. Nothing should be scored."""


def cookie_header(jar_text):
    """The Cookie header for every cookie in a jar curl wrote. curl marks an
    HttpOnly cookie with a "#HttpOnly_" prefix, which a cookie-file reader takes
    for a comment, and the session cookie is Secure, which a client refuses to
    send over the lane's plain-HTTP localhost. So the jar is read here."""
    pairs = []
    for line in jar_text.splitlines():
        line = line.removeprefix("#HttpOnly_")
        fields = line.split("\t")
        if line.startswith("#") or len(fields) != 7:
            continue
        pairs.append(f"{fields[5]}={fields[6]}")
    return "; ".join(pairs)


class Admin:
    def __init__(self, base, cookie_jar):
        with open(cookie_jar, encoding="utf-8") as jar:
            self._cookie = cookie_header(jar.read())
        if not self._cookie:
            raise SetupFault(f"the cookie jar {cookie_jar} holds no cookie, so the step has no session")
        self._base = base.rstrip("/") + "/v1"
        self._open = urllib.request.build_opener().open

    def call(self, method, path, body=None, content_type="application/json"):
        data = body if isinstance(body, bytes) or body is None else json.dumps(body).encode()
        request = urllib.request.Request(self._base + path, data=data, method=method)
        request.add_header("Cookie", self._cookie)
        if data is not None:
            request.add_header("Content-Type", content_type)
        try:
            with self._open(request, timeout=60) as response:
                return json.loads(response.read() or b"{}")
        except urllib.error.HTTPError as err:
            detail = err.read().decode("utf-8", "replace")[:300]
            raise SetupFault(f"{method} {path} answered HTTP {err.code}: {detail}") from err

    def upload(self, path, filename, content, content_type):
        boundary = uuid.uuid4().hex
        body = (
            f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\n"
            f"Content-Type: {content_type}\r\n\r\n"
        ).encode() + content + f"\r\n--{boundary}--\r\n".encode()
        return self.call("POST", path, body, f"multipart/form-data; boundary={boundary}")


# The three contacts whose cards resemble one already on file, each with
# the address the card states. The seed writes the contacts; the cards differ.
CARD_REVIEWS = (
    ("Wiebke Brandstätter", "w.brandstaetter@kaelte-nord.test"),
    ("Okko Rademacher", "okko.r@werft-post.test"),
    ("Theda Uphoff", "theda@uphoff-privat.test"),
)


def queue_problems(pending, wanted_names):
    """What is wrong with the pending queue a step meant to leave: a review it
    staged that is missing, or a held draft nothing in the step asked for."""
    summaries = [(row.get("kind", ""), row.get("summary", "")) for row in pending]
    problems = [
        f"no pending card review names {name}"
        for name in wanted_names
        if not any(kind == "vcard_create" and name in summary for kind, summary in summaries)
    ]
    problems += [f"a held draft is already waiting: {summary}" for kind, summary in summaries if kind == "held_draft"]
    return problems


def _pending(admin):
    return admin.call("GET", "/approvals?status=pending&limit=100").get("data", [])


def card_reviews_and_recap(admin):
    """Three unrelated card reviews in the admin's queue, and the post-meeting
    recap on with the admin as its owner, so a meeting logged afterwards with a
    contact who has an email stages a held draft against that meeting."""
    for name, address in CARD_REVIEWS:
        card = f"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:{name}\r\nEMAIL;TYPE=WORK:{address}\r\nEND:VCARD\r\n"
        report = admin.upload("/contacts/vcard-import", "card.vcf", card.encode(), "text/vcard")
        outcomes = [row.get("outcome") for row in report.get("results", [])]
        if outcomes != ["needs_review"]:
            raise SetupFault(f"the card for {name} was meant to wait for review; the import answered {report}")

    rules = admin.call("GET", "/automations?limit=100").get("data", [])
    recap = next((rule for rule in rules if rule.get("key") == "post_meeting_recap"), None)
    if recap is None:
        raise SetupFault("the workspace has no post-meeting recap rule")
    users = admin.call("GET", f"/users?q={ADMIN_EMAIL}").get("data", [])
    if not users:
        raise SetupFault(f"no user answers to {ADMIN_EMAIL}, so the recap has no owner to check")
    me = users[0]["id"]
    enabled = admin.call("PATCH", f"/automations/{recap['id']}", {"status": "enabled"})
    if enabled.get("status") != "enabled" or enabled.get("owner_id") != me:
        raise SetupFault(f"enabling the post-meeting recap as {ADMIN_EMAIL} answered {enabled}")

    problems = queue_problems(_pending(admin), [name for name, _ in CARD_REVIEWS])
    if problems:
        raise SetupFault("; ".join(problems))


STEPS = {"card_reviews_and_recap": card_reviews_and_recap}


def main(argv):
    if len(argv) == 3 and argv[1] == "--known":
        if argv[2] in STEPS:
            return 0
        print(f"no setup step is named {argv[2]!r}; known: {', '.join(sorted(STEPS))}", file=sys.stderr)
        return 1
    if len(argv) != 4 or argv[1] not in STEPS:
        print(__doc__, file=sys.stderr)
        return 1
    try:
        STEPS[argv[1]](Admin(argv[2], argv[3]))
    except (SetupFault, OSError, ValueError) as fault:
        print(f"setup {argv[1]} failed: {fault}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
