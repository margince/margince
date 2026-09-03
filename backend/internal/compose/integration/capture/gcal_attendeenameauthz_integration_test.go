// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The name on an invitation is written by whoever sent it, and they are outside
// this workspace.
//
// That is the whole reason the naming path takes a visibility probe. An
// organizer chooses both the attendee list and the display name beside each
// address, so without one they could put a colleague's contact on a meeting,
// type a plausible name for them, and have it stick — on a record they can
// neither see nor reach. Filing a meeting under a record puts a message on a
// page; naming one writes what the record IS called.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedUnreachablePerson writes a contact owner-scoped to a rep on the OTHER
// team, through the same columns capture writes. `visibility = 'owner'` is what
// platform/auth reads as capture privacy: the row is invisible to every
// principal but its owner, an admin included.
func seedUnreachablePerson(t *testing.T, e *integration.SearchEnv, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person (id, full_name, source, captured_by, owner_id, visibility)
			VALUES ($1, 'Buyer', 'manual', 'human:x', $2, 'owner')`, id, e.Rep3); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:x')`, id, email)
		return err
	}); err != nil {
		t.Fatalf("seeding the unreachable contact: %v", err)
	}
	return id
}

// An organizer naming a contact the capturing seat cannot see writes nothing.
// The meeting still captures — a name is not worth losing a message over — and
// the contact keeps the name it had.
func TestAnInvitationCannotNameAContactTheSeatCannotSee(t *testing.T) {
	e := integration.SetupSearch(t)
	hidden := seedUnreachablePerson(t, e, "buyer@acme.com")

	syncOneGcalMeeting(t, e)

	if _, _, full := personName(t, e, hidden); full != "Buyer" {
		t.Errorf("the contact is now called %q — an organizer outside the workspace named a record this seat cannot reach", full)
	}
}
