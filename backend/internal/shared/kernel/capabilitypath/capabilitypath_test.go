// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capabilitypath

import (
	"strings"
	"testing"
)

// These cases pin the redaction from both sides: the credential never
// survives, and everything an operator reads a log FOR — route, trailing
// verb, ordinary record ids — still does.
func TestRedactHidesTheCredentialAndKeepsTheRoute(t *testing.T) {
	// Deliberately a sentence rather than a realistic token: the assertion
	// is that this segment does not reach the line, and a fixture that LOOKS
	// like a credential is one every secret scanner has to be told about
	// forever after.
	const token = "this-stands-in-for-a-capability-token"

	for _, tc := range []struct {
		name, path, want string
	}{
		{
			"the preference token segment itself",
			"/v1/public/preferences/" + token,
			"/v1/public/preferences/[redacted]",
		},
		{
			"a trailing verb survives",
			"/v1/public/preferences/" + token + "/unsubscribe",
			"/v1/public/preferences/[redacted]/unsubscribe",
		},
		{
			"the confirm token opens a record, so it goes too",
			"/v1/public/confirm/" + token,
			"/v1/public/confirm/[redacted]",
		},
		{
			"an unrelated path is untouched",
			"/v1/deals/018f2a10-0000-7000-8000-000000000001",
			"/v1/deals/018f2a10-0000-7000-8000-000000000001",
		},
		{
			"the prefix with nothing after it has nothing to hide",
			"/v1/public/preferences/",
			"/v1/public/preferences/",
		},
		{
			// The booking slug is the only admission check on a route that
			// creates a person, records a consent grant and books a meeting.
			// The host publishing the URL is not the host publishing it to a
			// log aggregator.
			"a booking slug admits a write, so it goes too",
			"/v1/public/booking/acme-discovery-call",
			"/v1/public/booking/[redacted]",
		},
		{
			// The availability read hangs off the same slug, and the verb
			// after it is what makes the line worth keeping.
			"the availability verb survives the slug",
			"/v1/public/booking/acme-discovery-call/availability",
			"/v1/public/booking/[redacted]/availability",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.path); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// Every prefix must end in a slash, or Redact eats the wrong segment: a
// prefix of "/v1/public/confirm" would match "/v1/public/confirmation/x" and
// redact a route that carries no credential at all.
func TestEveryCredentialPrefixEndsInASlash(t *testing.T) {
	for _, prefix := range CredentialPrefixes() {
		if !strings.HasSuffix(prefix, "/") {
			t.Errorf("credential prefix %q does not end in a slash, so it matches sibling routes by prefix", prefix)
		}
	}
}

// The exported list is what a gate reads to check the routes; handing out
// the backing array would let a caller empty the redaction for the whole
// process.
func TestCredentialPrefixesCannotBeMutatedByItsCaller(t *testing.T) {
	got := CredentialPrefixes()
	if len(got) == 0 {
		t.Fatal("no credential prefixes are declared, so nothing is ever redacted")
	}
	got[0] = "/nonsense/"

	if CredentialPrefixes()[0] == "/nonsense/" {
		t.Error("a caller's write reached the package's own list")
	}
}
