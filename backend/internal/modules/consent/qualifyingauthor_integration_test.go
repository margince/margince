// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Whether being COPIED on somebody else's message makes correspondence lawful.
//
// It does not, and the module already says so twice — authorizeevidence.go's
// authorIsTheSubject requires role 'from', and its own comment explains that
// counting a Cc "would let anyone create a lawful basis for writing to a third
// party by putting them in Cc".
//
// The Art 6(1)(f) arm underneath the send gate did not ask. It read
// activity_link, which is a FILING and carries no author concept, and that was
// harmless only while a message was filed under the party the ladder judged.
// Capture now files a message under every participant it resolves
// (capture/sinkmaillinks.go), so the filing stopped implying authorship — and
// without this the cc'd stranger becomes writable-to.
//
// An integration test because the defect is in a SQL join: a unit test over the
// Go around it passes with either query.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// filedInbound plants an inbound message filed under the subject, naming them
// in the given participant role — or in none at all when role is empty.
//
// Hand-inserted on purpose, unlike the fixtures that seed through a production
// writer. What is under test is what the READ counts, and the read must refuse
// this row's shape whatever wrote it: a caller may post an activity with
// direction=inbound and a link to any contact they can read, so the shape is
// reachable without capture being involved at all.
func (e *qualifyingEnv) filedInbound(t *testing.T, role string) {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
			VALUES ($1, 'email', 'inbound', 'gmail', now(), 'connector:gmail:x')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, e.person); err != nil {
			return err
		}
		if role == "" {
			return nil
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, person_id, role)
			VALUES ($1, $2, $3)`, id, e.person, role)
		return err
	}); err != nil {
		t.Fatalf("planting an inbound message filed under the person (role %q): %v", role, err)
	}
}

// Being cc'd on a message somebody else wrote authorizes nothing.
//
// Mutation: drop the activity_participant join from inboundQualifyingEvent and
// this passes — the verdict turns allowed on a message the person only received.
func TestBeingCopiedOnAMessageDoesNotMakeCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "cc")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) after the person was merely cc'd, want not allowed — "+
			"a filing link says the message belongs on their record, never that they wrote it, "+
			"and counting it lets anyone manufacture a basis for writing to a third party by "+
			"putting them in Cc", got.State, got.Reason)
	}
}

// A message filed under somebody who is named in NO participant row authorizes
// nothing either. This is the same hole reached without a Cc: the link alone.
func TestAMessageFiledUnderSomebodyWhoWroteNothingAuthorizesNothing(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a message the person is only FILED under, want not allowed — "+
			"activity_link carries no author concept, so a caller who may file an activity under a "+
			"contact could otherwise write their own evidence for mailing them", got.State, got.Reason)
	}
}

// And the arm still answers for the person who actually wrote to us — the case
// the whole Art 6(1)(f) reading exists for.
//
// Without this the two refusals above are satisfied by an arm that refuses
// everybody, which is the failure mode a default-deny gate hides best.
func TestTheAuthorOfAnInboundMessageStillMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "from")

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for the person who WROTE to us, want allowed — "+
			"they started the correspondence, which is the whole Art 6(1)(f) reading",
			got.State, got.Reason)
	}
	if got.Qualifying == nil || got.Qualifying.Kind != "inbound_message" {
		t.Errorf("qualifying event %+v, want an inbound_message — the basis has to name the "+
			"message it was read from, or nothing can be stamped for Art 5(2)", got.Qualifying)
	}
}
