// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The edit-scope rule as a table: an edit corrects what a staged action SAYS,
// never which record it applies to.
func TestAssertSameEntityRefsPinsEveryRecordTheProposalNames(t *testing.T) {
	const (
		mine   = "8bf1b0f2-6c2c-4a1a-9a0e-1c9b2a3d4e5f"
		theirs = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
		alice  = "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"
	)

	tests := []struct {
		name        string
		original    string
		edited      string
		wantChanged []string
	}{
		{
			name:     "editing the content a human is meant to correct is allowed",
			original: `{"company_id":"` + mine + `","proposed_name":"Acme","contacts":["` + alice + `"]}`,
			edited:   `{"company_id":"` + mine + `","proposed_name":"Acme GmbH","contacts":["` + alice + `"]}`,
		},
		{
			name:     "a payload naming no record at all is entirely editable",
			original: `{"stage":"proposal","note":"agent version"}`,
			edited:   `{"stage":"won","note":"human version","reason":"signed"}`,
		},
		{
			name:        "repointing the target at another record is refused",
			original:    `{"company_id":"` + mine + `","proposed_name":"Acme"}`,
			edited:      `{"company_id":"` + theirs + `","proposed_name":"Acme"}`,
			wantChanged: []string{"/company_id"},
		},
		{
			name:        "dropping the reference is refused too — an absent id resolves to nothing the gate checked",
			original:    `{"company_id":"` + mine + `","proposed_name":"Acme"}`,
			edited:      `{"proposed_name":"Acme"}`,
			wantChanged: []string{"/company_id"},
		},
		{
			name:        "introducing a reference the staging never carried is refused",
			original:    `{"proposed_name":"Acme"}`,
			edited:      `{"proposed_name":"Acme","owner_id":"` + theirs + `"}`,
			wantChanged: []string{"/owner_id"},
		},
		{
			name:        "a reference nested in a list is pinned like a top-level one",
			original:    `{"contacts":["` + alice + `"]}`,
			edited:      `{"contacts":["` + theirs + `"]}`,
			wantChanged: []string{"/contacts/[0]"},
		},
		{
			name:        "a reference nested in an object is pinned like a top-level one",
			original:    `{"link":{"activity_id":"` + alice + `"}}`,
			edited:      `{"link":{"activity_id":"` + theirs + `"}}`,
			wantChanged: []string{"/link/activity_id"},
		},
		// The editor chooses the key names, so it can try to spell a nested
		// path as one flat key and have the two read as the same location.
		// They must not: the reference would move out of where the effect
		// reads it while this check saw nothing change.
		// THE PREMISE assertSameCallIdentity EXISTS FOR, asserted rather than
		// assumed. entityRefs collects only strings that parse WHOLLY as a
		// UUID, so a record id inside a request path is invisible to it and a
		// re-aimed call reads here as no change at all. If this ever started
		// failing — entityRefs widened to find ids inside strings — the second
		// assertion beside it would look redundant and become a candidate for
		// deletion, with nothing left to say why it is not.
		{
			name:     "a record named inside a REST path is invisible here, which is why the call identity is pinned separately",
			original: `{"operation":"advanceDeal","path":"/v1/deals/` + mine + `/advance","body":{}}`,
			edited:   `{"operation":"advanceDeal","path":"/v1/deals/` + theirs + `/advance","body":{}}`,
		},
		{
			name:        "a flat key spelling a nested path does not collide with it",
			original:    `{"link":{"activity_id":"` + alice + `"}}`,
			edited:      `{"link/activity_id":"` + alice + `"}`,
			wantChanged: []string{"/link/activity_id", "/link~1activity_id"},
		},
		{
			name:        "an object key spelling an array index does not collide with it",
			original:    `{"contacts":["` + alice + `"]}`,
			edited:      `{"contacts":{"[0]":"` + alice + `"}}`,
			wantChanged: []string{"/contacts/[0]", "/contacts/~20]"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := assertSameEntityRefs(json.RawMessage(tc.original), json.RawMessage(tc.edited))
			if len(tc.wantChanged) == 0 {
				if err != nil {
					t.Fatalf("edit refused: %v — a human must stay free to correct the action's content", err)
				}
				return
			}
			var retargeted *RetargetedEditError
			if !errors.As(err, &retargeted) {
				t.Fatalf("edit accepted (err = %v), want RetargetedEditError naming %v", err, tc.wantChanged)
			}
			if strings.Join(retargeted.Paths, ",") != strings.Join(tc.wantChanged, ",") {
				t.Errorf("refused paths = %v, want %v", retargeted.Paths, tc.wantChanged)
			}
		})
	}
}

// A REST staging carries its record inside the request PATH, not as a bare field.
// entityRefs collects only strings that parse wholly as a UUID, so the id in
// "/v1/deals/<uuid>/advance" is invisible to it — and an edit that rewrites the
// path therefore looks like a content correction while it re-aims the effect at a
// different record. The version pin still re-reads the ORIGINAL target, so nothing
// downstream notices either.
func TestAnEditMayNotRepointTheRecordNamedInARestPath(t *testing.T) {
	staged := ids.NewV7()
	other := ids.NewV7()
	// toStageID is fixed across both calls so the ONLY thing that differs
	// between the staged and edited payload is the record named in the path.
	// A body id that varied too would be refused as a changed `/body`, and the
	// case would pass without ever showing that the PATH is pinned — which is
	// the whole claim, since the path is where a REST staging keeps its record.
	toStageID := ids.NewV7().String()
	rest := func(id ids.UUID) json.RawMessage {
		return json.RawMessage(`{"operation":"advanceDeal","path":"/v1/deals/` + id.String() +
			`/advance","body":{"to_stage_id":"` + toStageID + `"}}`)
	}
	retargeted := requireRetargeted(t, assertSameCallIdentity(rest(staged), rest(other)),
		"an edit that moved the call from one deal to another was accepted; the approving "+
			"human judged the first record and the effect would land on the second")
	if strings.Join(retargeted.Paths, ",") != "/path" {
		t.Errorf("refused paths = %v, want [/path]", retargeted.Paths)
	}
}

// requireRetargeted fails the test with msg if err is not a *RetargetedEditError,
// and returns it otherwise — shared by every call-identity case below that also
// needs to inspect WHICH member the refusal names.
func requireRetargeted(t *testing.T, err error, msg string) *RetargetedEditError {
	t.Helper()
	var retargeted *RetargetedEditError
	if !errors.As(err, &retargeted) {
		t.Fatalf("%s (err = %v)", msg, err)
	}
	return retargeted
}

// The call-identity rule as a table: an edit may rewrite `body`, never any
// other top-level member of a REST staging — pinned by EXCLUDING body rather
// than by naming "operation" and "path", so a member the canonical call adds
// tomorrow (an If-Match or Idempotency-Key header, sibling of body) is pinned
// by construction rather than by someone remembering to list it.
func TestAssertSameCallIdentityPinsEveryMemberOfTheStagedCallExceptBody(t *testing.T) {
	const dealPath = `/v1/deals/11111111-1111-4111-8111-111111111111/advance`

	tests := []struct {
		name        string
		original    string
		edited      string
		wantChanged []string
	}{
		{
			name:     "content stays editable — that is what ADR-0036 §4 is for",
			original: `{"operation":"advance_deal","path":"` + dealPath + `","body":{"note":"as discussed"}}`,
			edited:   `{"operation":"advance_deal","path":"` + dealPath + `","body":{"note":"as agreed on the call"}}`,
		},
		{
			// A tool staging carries neither operation nor path. It must pass
			// rather than fail closed here, or every MCP-staged approval
			// becomes uneditable — entityRefs governs its content instead.
			name:     "a tool staging (no operation or path) has no call identity to pin",
			original: `{"deal_id":"11111111-1111-4111-8111-111111111111","note":"a"}`,
			edited:   `{"deal_id":"11111111-1111-4111-8111-111111111111","note":"b"}`,
		},
		{
			name:        "repointing path is refused",
			original:    `{"operation":"advance_deal","path":"` + dealPath + `","body":{}}`,
			edited:      `{"operation":"advance_deal","path":"/v1/deals/other/advance","body":{}}`,
			wantChanged: []string{"/path"},
		},
		{
			// operation names the call as much as path names the record — a
			// staging table-driven only on path would leave this arm of the
			// same guard unexercised.
			name:        "repointing operation alone is refused",
			original:    `{"operation":"advance_deal","path":"` + dealPath + `","body":{}}`,
			edited:      `{"operation":"disqualify_lead","path":"` + dealPath + `","body":{}}`,
			wantChanged: []string{"/operation"},
		},
		{
			// Dropping a member is a change, not an absence: an edit that
			// deletes `path` leaves a payload the redemption re-derives its
			// own path for, which is the same re-aiming by another route.
			name:        "dropping path is a retarget, not an absence",
			original:    `{"operation":"advance_deal","path":"` + dealPath + `","body":{}}`,
			edited:      `{"operation":"advance_deal","body":{}}`,
			wantChanged: []string{"/path"},
		},
		{
			// A member the canonical call does not carry TODAY is still
			// pinned: deny-by-default means this needs no update when a
			// future member is added — proven with an arbitrary name here
			// precisely so no real member has to be named for the rule to
			// hold.
			name:        "an unrecognized top-level member is pinned exactly like operation and path",
			original:    `{"operation":"advance_deal","path":"` + dealPath + `","if_match":"7","body":{}}`,
			edited:      `{"operation":"advance_deal","path":"` + dealPath + `","if_match":"9","body":{}}`,
			wantChanged: []string{"/if_match"},
		},
		{
			// The cross-task seam: compose.canonicalRESTCall writes a
			// `headers` member carrying Idempotency-Key, the caller's retry
			// key. Nothing else in this codebase asserts that member is
			// pinned; this case proves the deny-by-default rule above
			// already covers the REAL member canonicalRESTCall adds, not
			// only the placeholder name the case above uses.
			name:        "editing headers (Idempotency-Key) is refused, not treated as content",
			original:    `{"operation":"advance_deal","path":"` + dealPath + `","headers":{"Idempotency-Key":"k1"},"body":{}}`,
			edited:      `{"operation":"advance_deal","path":"` + dealPath + `","headers":{"Idempotency-Key":"k2"},"body":{}}`,
			wantChanged: []string{"/headers"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := assertSameCallIdentity(json.RawMessage(tc.original), json.RawMessage(tc.edited))
			if len(tc.wantChanged) == 0 {
				if err != nil {
					t.Fatalf("edit refused: %v — a human must stay free to correct the action's content", err)
				}
				return
			}
			retargeted := requireRetargeted(t, err, "edit accepted, want RetargetedEditError naming "+strings.Join(tc.wantChanged, ","))
			if strings.Join(retargeted.Paths, ",") != strings.Join(tc.wantChanged, ",") {
				t.Errorf("refused paths = %v, want %v", retargeted.Paths, tc.wantChanged)
			}
		})
	}
}

// The refusal message names the field, so an operator reading a 422 can tell a
// typo from an attempt to re-aim the approval.
func TestRetargetedEditErrorNamesTheOffendingPaths(t *testing.T) {
	err := &RetargetedEditError{Paths: []string{"/company_id", "/owner_id"}}
	msg := err.Error()
	for _, want := range []string{"/company_id", "/owner_id"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not name %q", msg, want)
		}
	}
}

// A body-only edit survives a staged path carrying an HTML-significant
// character.
//
// The two sides of the comparison are spelled differently by construction:
// `before` comes back from a jsonb column, which emits `&`, `<` and `>` raw,
// and `after` comes from diffhash.Canonical through json.Marshal, which escapes
// them. Compared as bytes they never matched, so every later correction to the
// body was refused as a retarget — a legitimate edit the human is entitled to
// make, refused for a reason nothing in the message could explain.
//
// It failed CLOSED, so nothing was admitted that should not have been; what it
// cost was the edit.
func TestABodyEditSurvivesAPathPostgresAndGoSpellDifferently(t *testing.T) {
	// As jsonb hands it back: the ampersand RAW.
	staged := json.RawMessage(`{"operation":"listDeals","path":"/v1/deals?q=a&b","body":{"note":"first"}}`)
	// As json.Marshal writes it: the same path, the ampersand ESCAPED. This is
	// the difference — written raw on both sides the case passes against a
	// byte comparison and proves nothing.
	edited := json.RawMessage(`{"operation":"listDeals","path":"/v1/deals?q=a\u0026b","body":{"note":"corrected"}}`)

	if err := assertSameCallIdentity(staged, edited); err != nil {
		t.Fatalf("a body-only edit was refused as %v — the path is the SAME path, spelled by two encoders", err)
	}
}

// And the control, one character apart: a path that genuinely differs is still
// refused. Without it the case above would pass against a comparison that had
// stopped comparing.
func TestAPathThatGenuinelyDiffersIsStillRefused(t *testing.T) {
	staged := json.RawMessage(`{"operation":"listDeals","path":"/v1/deals?q=a&b","body":{}}`)
	edited := json.RawMessage(`{"operation":"listDeals","path":"/v1/deals?q=a&c","body":{}}`)

	retargeted := requireRetargeted(t, assertSameCallIdentity(staged, edited),
		"an edit that changed the path was accepted")
	if strings.Join(retargeted.Paths, ",") != "/path" {
		t.Errorf("refused paths = %v, want [/path]", retargeted.Paths)
	}
}

// A staged payload that is not an object is the approval's problem, not the
// server's.
//
// `proposed_change` is jsonb, which permits an array or a scalar. No producer
// stages one today — the column is what makes it reachable — and answering a
// bare error made it a 500, which tells a human their approval hit a server
// fault when what happened is that the staging is unusable.
func TestANonObjectStagedChangeIsRefusedAsAnInvalidEditNotAServerFault(t *testing.T) {
	for _, tc := range []struct {
		name             string
		staged, editedTo string
	}{
		{"an array", `[1,2,3]`, `{"body":{}}`},
		{"a scalar", `"just a string"`, `{"body":{}}`},
		{"an edit that is not an object", `{"operation":"x","path":"/v1/y","body":{}}`, `[1,2,3]`},
		// The null cases are the ones a decode-only check cannot see: `null`
		// unmarshals into a NIL map and returns no error, so a decode alone
		// answers "object" for it. Both wrong answers it produced are here.
		// Against a REST staging the empty side made every member read as
		// removed, so an unreadable payload came back as a retarget — a
		// refusal that names a cause the human cannot act on.
		{"a null staging", `null`, `{"operation":"x","path":"/v1/y","body":{}}`},
		{"a null edit", `{"operation":"x","path":"/v1/y","body":{}}`, `null`},
		// And null on both sides is the silent one: two empty member sets,
		// isRESTStaging false for each, so the guard returned nil and let the
		// edit through untouched.
		{"null on both sides", `null`, `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := assertSameCallIdentity(json.RawMessage(tc.staged), json.RawMessage(tc.editedTo))
			var invalid *InvalidEditError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v (%T), want an *InvalidEditError — anything else reaches writeErr as "+
					"neither refusal type and answers 500", err, err)
			}
		})
	}
}
