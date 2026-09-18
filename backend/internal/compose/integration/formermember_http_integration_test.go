// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A seat for somebody who already left.
//
// The HubSpot import carries work by 76 colleagues and 36 of them had gone before
// this installation held any of it. Their names arrive as free text on imported
// activities, and a name without a seat cannot be resolved, linked, or told
// apart from two others spelled the same way.
//
// The four properties below are what make such a seat safe to create in bulk,
// and each is a thing that would otherwise be a surprise: it costs no licence,
// it cannot be signed into, it stays out of the pickers, and it is recorded
// once.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// fullSeatsInUse counts what the licence meters, over the harness's owner
// connection rather than through identity.SeatUsage.
//
// The store read takes a principal carrying `license` or `seat_usage`, and this
// suite's harness builds HTTP callers rather than principals — so asking it
// through Go would mean inventing a context shape no other test in this package
// uses. The predicate is the one fullSeatsInUseQuery applies, kept here in the
// same words so a change to the metering rule shows up as a diff against this
// line rather than as a test that quietly stops measuring anything.
func fullSeatsInUse(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM app_user
		 WHERE seat_type = 'full' AND NOT is_agent
		   AND status NOT IN ('suspended', 'deactivated')`).Scan(&n); err != nil {
		t.Fatalf("counting the seats in use: %v", err)
	}
	return n
}

func TestAFormerMemberIsASeatNobodyCanEnterAndNobodyPaysFor(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	before := fullSeatsInUse(t, e)
	// A zero would pass the assertion below while proving the meter never ran:
	// if the predicate counted nothing at all, before and after would both be 0
	// and "the count did not move" would be true and meaningless. The harness
	// bootstraps an admin, so the meter has something to see. The sibling seat
	// suite guards its own read the same way.
	if before < 1 {
		t.Fatalf("seats in use = %d before the call, want at least the bootstrap admin — "+
			"a meter reading zero makes the assertion below vacuous", before)
	}

	var former userWire
	if status := e.Call(t, "POST", "/v1/users/former", map[string]any{
		"email": "Mutaz@Gradion.test", "display_name": "Mutaz Suleiman",
		"left_at": "2025-04-30T00:00:00Z", "source": "hubspot-mirror-2026-09-17",
	}, nil, &former); status != http.StatusCreated {
		t.Fatalf("recording a former member -> %d, want 201", status)
	}

	// DEACTIVATED on arrival, which is the one status that may never sign in.
	// Not `invited`: an invitation is a seat waiting to be entered, and there is
	// no token, no mail and nobody coming back.
	if former.Status != "deactivated" {
		t.Errorf("the former member's status is %q, want deactivated — a seat that can be "+
			"entered is not what recording a departed colleague creates", former.Status)
	}
	if former.Email != "mutaz@gradion.test" {
		t.Errorf("the email reads %q, want it lowercased like every other seat", former.Email)
	}
	// Defaulted rather than refused: the caller named no role, and what somebody
	// WAS is what this records.
	assertRoles(t, "former member", former, "rep")

	// NO LICENCE. fullSeatsInUseQuery counts every status but suspended and
	// deactivated, so thirty-six of these cost nothing — which is what makes
	// naming your own history bookkeeping rather than a purchase.
	if after := fullSeatsInUse(t, e); after != before {
		t.Errorf("seats in use moved %d -> %d — a departed colleague must not consume a "+
			"licence, or recording the 36 this import carries would cost 36 seats", before, after)
	}

	// OUT OF THE ROSTER by default, so nobody is offered a departed colleague as
	// an assignee or a recipient.
	var roster userListWire
	if status := e.Call(t, "GET", "/v1/users", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("listing the roster -> %d, want 200", status)
	}
	for _, u := range roster.Data {
		if u.ID == former.ID {
			t.Errorf("the default roster offers %q, who has left — the picker must not", u.DisplayName)
		}
	}

	// BUT FINDABLE when an admin asks, which is how they are reactivated if they
	// come back.
	var widened userListWire
	if status := e.Call(t, "GET", "/v1/users?include_inactive=true", nil, nil, &widened); status != http.StatusOK {
		t.Fatalf("listing with include_inactive -> %d, want 200", status)
	}
	var found bool
	for _, u := range widened.Data {
		if u.ID == former.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("include_inactive did not list the former member — an admin could never " +
			"reactivate somebody who came back")
	}

	// RECORDED ONCE. A second attempt is the wrong act: if they are merely
	// deactivated they already are what this route would create.
	var dupe refusalWire
	if status := e.Call(t, "POST", "/v1/users/former", map[string]any{
		"email": "mutaz@gradion.test", "display_name": "Mutaz Suleiman",
	}, nil, &dupe); status != http.StatusConflict {
		t.Fatalf("recording the same colleague twice -> %d, want 409", status)
	}
	assertActionableRefusal(t, "duplicate former member", dupe, "email_taken")
}

// The contract bounds `source` at 200 characters and the generated wrapper
// enforces no maxLength, so the handler is the only thing between a caller and
// an unbounded string riding into the audit row's `after` image. It is an
// operator's label — "hubspot-mirror-2026-09-17" — not content, and nothing
// downstream truncates it.
//
// CHARACTERS, not bytes, which is why the over-long value below is built from
// a multi-byte letter: at 201 runes it is 402 bytes, and a byte-counting bound
// would refuse it for the wrong reason while passing a 200-rune accented label
// that the contract permits.
func TestAFormerMemberSourceIsBoundedTheWayTheContractSaysItIs(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var refused refusalWire
	if status := e.Call(t, "POST", "/v1/users/former", map[string]any{
		"email": "toolong@gradion.test", "display_name": "Source Too Long",
		"source": strings.Repeat("ä", 201),
	}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("a 201-character source -> %d, want 422", status)
	}
	// `validation_error` rather than `length`: httperr.Validation carries the
	// field and its reason in the body, and stamps ONE wire code for every
	// field refusal. A client branches on the code and reads the field from the
	// detail, so asserting the reason here would be asserting a string this
	// surface does not put on the wire.
	assertActionableRefusal(t, "over-long source", refused, "validation_error")

	// The positive control, without which the assertion above would pass on a
	// route that refused every source: 200 runes of the same letter is exactly
	// the contract's limit and must be accepted.
	var accepted userWire
	if status := e.Call(t, "POST", "/v1/users/former", map[string]any{
		"email": "atlimit@gradion.test", "display_name": "Source At Limit",
		"source": strings.Repeat("ä", 200),
	}, nil, &accepted); status != http.StatusCreated {
		t.Fatalf("a 200-character source -> %d, want 201 — the bound counts characters, "+
			"and a byte-counting one would refuse a name the contract permits", status)
	}
}
