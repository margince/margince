// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A contact named by the invitation that named them.
//
// A person minted from a calendar notification addressed to a bare address is
// named by its local part: `chris@erlerventures.org` produced a contact called
// "Chris", unconfident, first and last name null. The event that carried the
// invitation names them in full — every provider sends the attendee's display
// name — and the whole chain from decode to person row dropped it.
//
// This runs the real gcal connector against the stubbed Google, through the real
// Registry and the ONE Sink, because what is under test spans four writes inside
// the capture transaction: the decode that keeps the name, the participant row
// that records it, the seam that carries it to the people module, and the fill
// whose guards decide whether it is an improvement.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// personName reads what a person record is called: the split columns and the
// displayed name, which move together or not at all.
func personName(t *testing.T, e *integration.SearchEnv, id ids.UUID) (first, last, full string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(first_name, ''), coalesce(last_name, ''), full_name
			  FROM person WHERE id = $1`, id).Scan(&first, &last, &full)
	}); err != nil {
		t.Fatalf("reading the person's name: %v", err)
	}
	return first, last, full
}

// The sync-time ordering: the attendee is already a contact when the meeting
// lands, and the invitation completes the name their address could not.
func TestACalendarInvitationNamesAContactItsAddressCouldNot(t *testing.T) {
	e := integration.SetupSearch(t)

	// Through the real people store, with only the local part for a name AND
	// under a CONNECTOR principal — which together are what capture writes for
	// a person minted from a bare address.
	//
	// The principal is not incidental. captured_by records who minted the row,
	// and completePersonName reads it: a person a human created is one whose
	// name a human is taken to have set, so the invitation must not overwrite
	// it (#3974). Seeding this under a human context said "a colleague typed
	// Buyer" while the comment claimed the capture shape, and the fill then
	// correctly refused — the test asserted the opposite of what its own setup
	// described.
	store := people.NewStore(e.DB())
	buyer, err := store.EnsurePersonByEmail(connectorCreator(e), "Buyer", "buyer@acme.com", "manual")
	if err != nil {
		t.Fatalf("seeding the attendee: %v", err)
	}
	if first, last, full := personName(t, e, buyer); first != "" || last != "" || full != "Buyer" {
		t.Fatalf("seeded person is %q/%q/%q, want the one-word name a bare address produces", first, last, full)
	}

	syncOneGcalMeeting(t, e)

	first, last, full := personName(t, e, buyer)
	if first != "Robin" || last != "Buyer" {
		t.Errorf("person is %q %q, want Robin Buyer — the invitation named them and the fill had both columns empty",
			first, last)
	}
	if full != "Robin Buyer" {
		t.Errorf("displayed name = %q, want %q — a record showing one word while its columns say two is a fill nobody can see",
			full, "Robin Buyer")
	}
}

// A name a human typed is never replaced by whatever an organizer spelled. The
// guard is completePersonName's, shared with the mail ladder, and this is the
// case that proves the calendar path did not route around it.
func TestAnInvitationNeverRenamesAContactAHumanNamed(t *testing.T) {
	e := integration.SetupSearch(t)

	store := people.NewStore(e.DB())
	buyer, err := store.EnsurePersonByEmail(personCreator(e), "Robin Q. Buyer-Smith", "buyer@acme.com", "manual")
	if err != nil {
		t.Fatalf("seeding the attendee: %v", err)
	}

	syncOneGcalMeeting(t, e)

	if _, _, full := personName(t, e, buyer); full != "Robin Q. Buyer-Smith" {
		t.Errorf("displayed name = %q, want the name the human typed", full)
	}
}
