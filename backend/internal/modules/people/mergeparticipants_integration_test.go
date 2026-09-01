// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A merge moves who-was-in-it, not only how the message is filed.
//
// activity_link followed the survivor from the start; activity_participant did
// not, and nothing failed when it stayed behind. The rows kept naming a record
// no read returns, so the interaction graph — the answer to "who on our team
// knows this contact" — went on crediting the merged-away person forever, and
// every reader of those rows matches person_id exactly with none walking
// merged_into_id.
//
// This needs a database for the collision: uq_activity_participant spans
// (activity, role, user, person, address), so the case where BOTH halves of the
// merge sat on ONE activity in the same role is a constraint violation the unit
// lane cannot see.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedParticipantOn puts one person on one activity in a role, the way capture
// records who was in a conversation.
func (e *dedupeEnv) seedParticipantOn(ctx context.Context, t *testing.T, activity ids.ActivityID, person ids.PersonID, role, address string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var addr *string
		if address != "" {
			addr = &address
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (id, activity_id, person_id, role, address)
			VALUES ($1, $2, $3, $4, $5)`, ids.NewV7(), activity, person, role, addr)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// participantPeopleOn answers which people one activity's participant rows name.
func (e *dedupeEnv) participantPeopleOn(ctx context.Context, t *testing.T, activity ids.ActivityID) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT person_id FROM activity_participant
			  WHERE activity_id = $1 AND person_id IS NOT NULL ORDER BY person_id`, activity)
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
// activity, so after the merge that activity names ONE person once.
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
	e.seedParticipantOn(ctx, t, moved, retired.PersonID, "cc", "")

	// Another carries BOTH, in the same role and with the same other arms —
	// one party recorded twice, which is the uniqueness collision.
	shared := e.seedCapturedMail(ctx, t, "other@merge.test", "from")
	e.seedParticipantOn(ctx, t, shared, survivor.PersonID, "cc", "")
	e.seedParticipantOn(ctx, t, shared, retired.PersonID, "cc", "")

	if _, err := e.store.MergePerson(ctx, retired.PersonID, survivor.PersonID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := e.participantPeopleOn(ctx, t, moved); len(got) != 1 || got[0] != survivor.PersonID.UUID {
		t.Errorf("the moved activity names %v, want the survivor %s — a participant row left behind credits a record no read returns",
			got, survivor.PersonID)
	}
	got := e.participantPeopleOn(ctx, t, shared)
	if len(got) != 1 || got[0] != survivor.PersonID.UUID {
		t.Errorf("the shared activity names %v, want the survivor once — the two halves were one party, not two", got)
	}
}
