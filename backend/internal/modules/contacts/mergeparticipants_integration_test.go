// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge moves who was in a message, and not only how the message is filed.
//
// Readers of activity_participant match contact_id and never walk merged_into_id.
// It needs a database for the collision. uq_activity_participant_identity spans
// activity, role and a digest of the identity columns.
// Both halves of a merge on one activity in one role violate it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedParticipantOn puts one contact on one activity in a role, the way capture
// records who was in a conversation.
func (e *dedupeEnv) seedParticipantOn(ctx context.Context, t *testing.T, activity ids.ActivityID, contact ids.ContactID, role, address string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var addr *string
		if address != "" {
			addr = &address
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (id, activity_id, contact_id, role, address)
			VALUES ($1, $2, $3, $4, $5)`, ids.NewV7(), activity, contact, role, addr)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// participantContactsOn answers which contacts one activity's participant rows name.
func (e *dedupeEnv) participantContactsOn(ctx context.Context, t *testing.T, activity ids.ActivityID) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT contact_id FROM activity_participant
			  WHERE activity_id = $1 AND contact_id IS NOT NULL ORDER BY contact_id`, activity)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// The ordinary case, plus the collision: the two halves sat on the same
// activity, so after the merge that activity names ONE contact once.
func TestAMergeMovesTheParticipantRowsOntoTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@merge.test", "Keeper", "merge.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@merge.test", "Dupe", "merge.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}

	// One activity carries only the retired record: it moves.
	moved := e.seedCapturedMail(ctx, t, "elsewhere@merge.test", "from")
	e.seedParticipantOn(ctx, t, moved, retired.ContactID, "cc", "")

	// Another carries BOTH, in the same role and with the same other arms —
	// one party recorded twice, which is the uniqueness collision.
	shared := e.seedCapturedMail(ctx, t, "other@merge.test", "from")
	e.seedParticipantOn(ctx, t, shared, survivor.ContactID, "cc", "")
	e.seedParticipantOn(ctx, t, shared, retired.ContactID, "cc", "")

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := e.participantContactsOn(ctx, t, moved); len(got) != 1 || got[0] != survivor.ContactID.UUID {
		t.Errorf("the moved activity names %v, want the survivor %s — a participant row left behind credits a record no read returns",
			got, survivor.ContactID)
	}
	got := e.participantContactsOn(ctx, t, shared)
	if len(got) != 1 || got[0] != survivor.ContactID.UUID {
		t.Errorf("the shared activity names %v, want the survivor once — the two halves were one party, not two", got)
	}
}

// A row stored before addresses were held lowercase moves with its lowercase
// twin. The two spellings become one row on the survivor, and the merge succeeds.
func TestAMergeFoldsTwoSpellingsOfTheSourcesAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@fold.test", "Keeper", "fold.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@fold.test", "Dupe", "fold.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}
	shared := e.seedCapturedMail(ctx, t, "other@fold.test", "from")
	testdb.WithParticipantAddressCheckLifted(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, contact_id, role, address)
			VALUES ($1, $2, 'cc', 'Dupe@Fold.test')`, shared, retired.ContactID)
		return err
	})
	e.seedParticipantOn(ctx, t, shared, retired.ContactID, "cc", "dupe@fold.test")

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID, nil); err != nil {
		t.Fatalf("merging a contact held under two spellings of one address: %v", err)
	}
	var rows int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM activity_participant
			 WHERE activity_id = $1 AND role = 'cc' AND contact_id = $2 AND address = 'dupe@fold.test'`,
			shared, survivor.ContactID).Scan(&rows)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("the survivor holds %d row(s) for the address, want the two spellings as one", rows)
	}
	if n := len(e.participantContactsOn(ctx, t, shared)); n != 1 {
		t.Errorf("the activity names %d contact row(s), want 1", n)
	}
}

// A merge whose context ended stops at the fold and reports it, before any
// participant row moves.
func TestAMergeReportsAFoldItCouldNotRun(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	stopped, stop := context.WithCancel(ctx)
	stop()
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return relinkParticipantRows(stopped, tx, ids.New[ids.ContactKind](), ids.New[ids.ContactKind]())
	})
	if err == nil {
		t.Fatal("moving participant rows under an ended context reported success")
	}
}
