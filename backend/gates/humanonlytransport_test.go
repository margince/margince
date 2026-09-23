// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// A human-only operation says so at the TRANSPORT, not only in the gate.
//
// The contract's preamble promises defence in depth: an operation marked
// `x-agent-access: human-only` also narrows the global `security:` so an agent
// bearer or Passport is not an accepted credential at all. The runtime gate
// (agentGate, held by agentgateinstalled_test.go) is the other half, and a
// reader is entitled to count on both.
//
// Ninety-six operations did not narrow anything. They inherited the global
// `[bearerAuth, cookieAuth]` — `inviteUser`, `changeUserRole`,
// `deactivateUser`, `issueUserPasswordLink`, `createRecordGrant`, the whole
// deal-room administration surface — so the sentence describing a
// transport-level defence was false over a fifth of the surface it described.
// Nothing noticed, because the overrides that did exist were added by hand one
// operation at a time.
//
// THREE CLASSES, because the rule is not one rule. What a human-only operation
// must declare depends on what authenticates its caller, which is a fact about
// the operation rather than a preference:
//
//   - authenticated by a human SESSION — the overwhelming majority. These
//     declare cookieAuth and nothing else, which is the promise.
//   - ANONYMOUS by design: the public booking page, the preference centre, the
//     one-click unsubscribe. The capability is a token in the URL and there is
//     no session to require; `security: []` is correct and cookieAuth would be
//     wrong.
//   - authenticated by the DEAL ROOM's own scheme, for the buyer who holds no
//     seat here.
//
// The last two are not gaps. They are why the preamble could not say "all"
// without being false, and each is ratified by name below rather than pattern-
// matched, so a new operation cannot join them by accident.
//
// WHAT THIS CANNOT SEE: whether the scheme a caller actually presents is the
// one declared. That is the runtime's, and agentGate is what answers it. This
// asks only that the document says what its own preamble says it says.

import (
	"fmt"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theHumanSessionScheme is the one an operation authenticated by a seat
// declares, and the only one that satisfies the preamble unaided.
const theHumanSessionScheme = "cookieAuth"

// humanOnlyWithoutASession ratifies the operations that are human-only and
// legitimately not cookie-authenticated, each with what DOES authenticate it.
//
// Listed, not derived, and that is the point: there is no property of these
// operations a pattern could read that a newly added one would not also carry.
// Joining this set is a decision about who may reach an endpoint holding no
// seat, and it should cost somebody an edit here.
var humanOnlyWithoutASession = gatekit.Waive(map[string]string{
	// The public booking page, opened by somebody who has never been here.
	"bookPublicMeeting": "the public booking page: an anonymous buyer holds no seat, and the link is the capability",

	// Pages a buyer opens from a link, holding no seat here. The token in the
	// URL is the capability, and requiring a session would make the page
	// unreachable by the very reader it is for.
	"publicStopContact":    "the public stop-contacting page: token-authed, POST-only, and the reader has no seat",
	"updatePreferences":    "the preference centre's write, authenticated by the token the link carried",
	"submitConfirmDetails": "the confirm-your-details page, same token, same reader with no seat",
	"oneClickUnsubscribe":  "RFC 8058 one-click unsubscribe, which a mail client POSTs on the reader's behalf with no session at all",

	// The buyer's Deal Room, before and around its own session.
	"requestDealRoomLink":        "the only self-service recovery a buyer has: asking for a fresh room link, answered 202 either way so it discloses nothing",
	"peekDealRoomCredential":     "answers only WHETHER a credential can still be exchanged, which the holder must be able to ask before they hold a session",
	"exchangeDealRoomCredential": "the exchange that MINTS the room session — requiring one would be circular",
	"openBuyerRoomThread":        "the buyer's own room, authenticated by dealRoomSession — the scheme that room mints for somebody who holds no seat here",
	"replyBuyerRoomThread":       "the same room's reply, under the same scheme",
	"signOutBuyerRoom":           "ending that session, which cannot require the session it ends to still be valid",

	// The login screen and the way back into it. Every one of these is reached
	// by somebody who by definition cannot present a session cookie.
	"requestPasswordReset": "asking for a reset mail, which is what somebody locked out does — there is no session to present",
	"resetPassword":        "redeeming that reset token, still before any session exists",
	"getAuthCapabilities":  "which authentication methods are operational, read by the login screen before anybody has signed in",
	"getAssistantProfile":  "the public identity and posture of the AI presence, shown before sign-in",
})

// aDealRoomSession is the scheme the buyer's own room authenticates with.
const aDealRoomSession = "dealRoomSession"

// securitySchemes names every scheme one operation's `security:` accepts.
//
// Read through the YAML tree rather than off the line, because the contract
// spells this list both ways — `security: [ { cookieAuth: [] } ]` on one line
// and a block sequence beneath `security:` on the next — and a check that knew
// only the flow form reported two operations as declaring nothing while they
// declared exactly the right thing. That is the direction a census must not
// fail in.
func securitySchemes(op map[string]any) ([]string, bool) {
	raw, declared := op["security"]
	if !declared {
		return nil, false
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, true
	}
	var schemes []string
	for _, entry := range entries {
		requirement, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		for name := range requirement {
			schemes = append(schemes, name)
		}
	}
	sort.Strings(schemes)
	return schemes, true
}

func TestEveryHumanOnlyOperationNarrowsItsTransport(t *testing.T) {
	t.Parallel()
	defer humanOnlyWithoutASession.AssertAllMatched(t)

	doc := loadContract(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("api/crm.yaml carries no paths, so this census would clear every operation by reading none")
	}
	// Counted so a PASS cannot come from a walk that found nothing: a renamed
	// annotation or a moved tree reads exactly like a compliant contract.
	humanOnly := 0
	for path, item := range paths {
		operations, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// An annotation written HERE rather than on an operation would shrink
		// this walk without failing it; refusePathLevelAgentAccess says why.
		refusePathLevelAgentAccess(t, path, operations)
		for method, raw := range operations {
			if !httpMethods[method] {
				continue
			}
			op, ok := raw.(map[string]any)
			if !ok || op["x-agent-access"] != "human-only" {
				continue
			}
			humanOnly++
			checkHumanOnlyTransport(t, fmt.Sprintf("%s %s", method, path), op)
		}
	}
	if humanOnly == 0 {
		t.Error("no operation in the contract declares x-agent-access: human-only, so this census proved nothing — " +
			"either the annotation was renamed or the walk no longer reaches the operations")
	}
}

// checkHumanOnlyTransport asks one operation for the declaration its class owes.
func checkHumanOnlyTransport(t *testing.T, where string, op map[string]any) {
	t.Helper()
	name, _ := op["operationId"].(string)
	schemes, declared := securitySchemes(op)

	if !declared {
		t.Errorf("%s (%s) is human-only and declares no `security:`, so it inherits the global "+
			"[bearerAuth, cookieAuth] — an agent bearer is an ACCEPTED credential at the transport, which is "+
			"the opposite of what the contract's preamble promises for this annotation.\n\tAdd "+
			"`security: [ { %s: [] } ]`, or ratify it in humanOnlyWithoutASession naming what authenticates "+
			"it instead.", where, name, theHumanSessionScheme)
		return
	}

	// An empty `security:` is "no authentication at all", which is right only
	// for the anonymous class and catastrophic anywhere else.
	if len(schemes) == 0 {
		if !humanOnlyWithoutASession.Waived(t, name) {
			t.Errorf("%s (%s) is human-only and declares `security: []`, which accepts every caller "+
				"unauthenticated.\n\tThat is correct only where the capability is a token in the URL and there is "+
				"no session to require. Ratify it in humanOnlyWithoutASession with what authenticates it, or give "+
				"it `%s`.", where, name, theHumanSessionScheme)
		}
		return
	}

	if len(schemes) == 1 && schemes[0] == theHumanSessionScheme {
		return
	}
	if len(schemes) == 1 && schemes[0] == aDealRoomSession && humanOnlyWithoutASession.Waived(t, name) {
		return
	}
	t.Errorf("%s (%s) is human-only and accepts %v.\n\tA human-only operation authenticated by a seat accepts "+
		"`%s` and nothing else — anything wider leaves an agent credential accepted at the transport. Narrow it, "+
		"or ratify the operation in humanOnlyWithoutASession naming what authenticates it.",
		where, name, schemes, theHumanSessionScheme)
}
