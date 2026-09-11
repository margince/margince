// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// THE BOTH-SIDES RULE, over a real Postgres.
//
// A dedupe pair names TWO records and the candidate row carries an evidence
// snapshot of each, so serving a pair IS a read of both. Every surface that
// speaks for the queue therefore owes the same rule — the lane that lists them,
// the read that opens one, and the BADGE that invites somebody to the lane.
//
// They are together in one file because they are one rule asked of three
// statements, each spelling its own clause. A case added for the lane that the
// badge never runs is exactly the gap these exist to close: before the badge had
// one, a count taken without the rule told a reader twelve and showed them
// three, and the number moved as records they could not open came and went.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// setRowVisibility runs one arm's privacy statement over a record: it either
// makes the row the owner's capture-private contact or releases it to the
// workspace (owner nil). Stated on both sides of a pair rather than assumed
// from what the create path stamped: the whole point of the test below is
// which side the caller can reach, so neither side's state may be incidental.
func setRowVisibility(ctx context.Context, t *testing.T, e *dedupeEnv, stmt string, owner *ids.UUID, id ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, stmt, owner, id)
		return err
	}); err != nil {
		t.Fatalf("setting the visibility of %s: %v", id, err)
	}
}

// halfVisibleArm is one entity type's turn through the queue's both-sides
// rule: how to leave an open pair of that type, how to make one of them the
// owner's capture-private record, and how to release the other.
type halfVisibleArm struct {
	entityType string
	setPrivate string
	setShared  string
	seed       func(context.Context, *testing.T, *dedupeEnv) (ids.UUID, ids.UUID)
}

// A dedupe pair names TWO records, and the queue surfaces it only when the
// caller can see BOTH — the candidate row carries the evidence snapshot of
// each side, so listing a pair IS a read of them.
//
// The reader that rule exists for is the half-visible one: the caller who can
// reach one side of the pair and not the other. Nothing else exercises the
// `AND` between the two EXISTS — a caller who sees both sides passes either
// way, and a caller who sees neither is refused by the first EXISTS alone.
// Swap that `AND` for an `OR` and every other test in this package still
// passes while the queue discloses the evidence of records the reader may not
// open.
//
// Run per entity type because the clause spells contact and company
// separately, and per SIDE because it spells left and right separately too.
// A lead has no arm here: it is workspace-readable identity with no capture
// privacy, so no human seat can ever see only half of a lead pair.
// halfVisibleArms is the entity vocabulary both half-visible cases run over —
// the list's and the count's. One builder because the two ask the same question
// of two different statements, and an arm added for one that the other never
// ran would leave exactly the gap this pair exists to close.
func halfVisibleArms() []halfVisibleArm {
	return []halfVisibleArm{
		{
			entityContact,
			`UPDATE contact SET owner_id = $1, visibility = 'owner' WHERE id = $2`,
			`UPDATE contact SET owner_id = $1, visibility = 'workspace' WHERE id = $2`,
			func(ctx context.Context, t *testing.T, e *dedupeEnv) (ids.UUID, ids.UUID) {
				return seedContactPair(ctx, t, e, "John Doe", "john@queue.test", "Jon Doe", "jon@queue.test", "queue.test")
			},
		},
		{
			entityCompany,
			`UPDATE company SET owner_id = $1, visibility = 'owner' WHERE id = $2`,
			`UPDATE company SET owner_id = $1, visibility = 'workspace' WHERE id = $2`,
			seedCompanyPair,
		},
	}
}

func TestDedupeQueueHidesAPairTheCallerCanOnlyHalfSee(t *testing.T) {
	for _, arm := range halfVisibleArms() {
		// The pair is stored canonically, lower id left (DH-DDL-1), and ids
		// are time-ordered — so hiding the record created first always probes
		// the clause's LEFT slot and never its right. Both get a turn: a
		// clause that reads one side's id twice still hides the pair from
		// callers who cannot see THAT side, and answers correctly for exactly
		// the half of them a one-sided probe happens to ask about.
		for _, hide := range []string{"first-created", "second-created"} {
			t.Run(arm.entityType+"/"+hide, func(t *testing.T) {
				halfVisiblePairStaysHidden(t, arm, hide)
			})
		}
	}
}

// THE BADGE OBEYS THE SAME RULE AS THE LANE, which is the half the list test
// above cannot see.
//
// CountOpenDedupeCandidates is what the daily digest reports and what the
// attention card badges, and it is a reader's invitation to open a queue. A
// count taken without the pair's both-sides rule is two defects at once: the
// reader is told twelve and shown three, and the number moves as records they
// may not open come and go — a weak existence oracle over somebody else's
// capture-private contact.
//
// Run over the same arms and both slots as the list, because the count assembles
// its clause separately and a slip in one spelling does not show in the other.
func TestTheOpenCountHidesAPairTheCallerCanOnlyHalfSee(t *testing.T) {
	for _, arm := range halfVisibleArms() {
		for _, hide := range []string{"first-created", "second-created"} {
			t.Run(arm.entityType+"/"+hide, func(t *testing.T) {
				e := setupDedupe(t)
				ctx := e.as()
				first, second := arm.seed(ctx, t, e)
				hidden, shared := first, second
				if hide == "second-created" {
					hidden, shared = second, first
				}
				setRowVisibility(ctx, t, e, arm.setPrivate, &e.rep, hidden)
				setRowVisibility(ctx, t, e, arm.setShared, nil, shared)

				// The owner of the private half reaches both sides, so the pair
				// is really there: without this the zero below would also be
				// what an empty queue answers.
				owner := e.asOwnScoped(e.rep)
				if open := countOpen(owner, t, e); open != 1 {
					t.Fatalf("the owner of the private side counts %d open, want 1 — "+
						"the other side is workspace-shared and reachable", open)
				}

				colleague := e.asOwnScoped(e.otherRep)
				if open := countOpen(colleague, t, e); open != 0 {
					t.Errorf("a caller who can see only half the pair is told %d duplicates are open — "+
						"they will be sent to a queue that shows none, and the number moves as records "+
						"they cannot open come and go", open)
				}
			})
		}
	}
}

// countOpen reads the badge through the production entry point.
func countOpen(ctx context.Context, t *testing.T, e *dedupeEnv) int {
	t.Helper()
	open, err := e.store.CountOpenDedupeCandidates(ctx)
	if err != nil {
		t.Fatalf("counting open candidates: %v", err)
	}
	return open
}

func halfVisiblePairStaysHidden(t *testing.T, arm halfVisibleArm, hide string) {
	t.Helper()
	e := setupDedupe(t)
	ctx := e.as()
	first, second := arm.seed(ctx, t, e)
	hidden, shared := first, second
	if hide == "second-created" {
		hidden, shared = second, first
	}
	seeded := openCandidates(ctx, t, e, arm.entityType)
	if len(seeded) != 1 {
		t.Fatalf("seeded %d %s candidates, want exactly 1", len(seeded), arm.entityType)
	}
	// Exactly one side becomes e.rep's capture-private record; the other is
	// released to the workspace, which every seat may read. So the colleague
	// below reaches one half of the pair and only the other is out of reach —
	// the state no other test in this package puts the queue in.
	setRowVisibility(ctx, t, e, arm.setPrivate, &e.rep, hidden)
	setRowVisibility(ctx, t, e, arm.setShared, nil, shared)

	// The private half's owner sees both halves, so the pair is still there
	// and still listable: the refusal below is the second EXISTS failing, not
	// an empty queue answering for free.
	owner := e.asOwnScoped(e.rep)
	if rows := openCandidates(owner, t, e, arm.entityType); len(rows) != 1 {
		t.Fatalf("the owner of the private side lists %d candidates, want 1 — the other side is workspace-shared and reachable", len(rows))
	}

	colleague := e.asOwnScoped(e.otherRep)
	if rows := openCandidates(colleague, t, e, arm.entityType); len(rows) != 0 {
		t.Fatalf("a caller who can see only half the pair lists %d candidates, want 0", len(rows))
	}
	if _, err := e.store.GetDedupeCandidate(colleague, seeded[0].ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("half-visible get = %v, want ErrNotFound — a pair the caller cannot fully read must not confirm it exists", err)
	}
}

// A disposition CHANGES both records the pair names — a dismissal suppresses
// them as a duplicate for the whole workspace, an undo puts the pair back — so
// each end carries write authority, exactly as the merge arm does through
// mergePair. The object grant is not that authority: contact and company
// are workspace-readable identity, so every seat holding contact:update passes
// the object gate over every colleague's records.
//
// The reader this exists for is the one who passes the READ gate and must fail
// the WRITE gate. That is why the pair is left at the default
// visibility='workspace': make it capture-private and GetDedupeCandidate
// answers 404 first, the probe under test never runs, and the test passes
// against a store that has no write gate at all.
func TestDedupeDispositionNeedsWriteAuthorityOverBothRecords(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedContactPair(ctx, t, e, "Ada Byron", "ada@authority.test", "Adah Byron", "adah@authority.test", "authority.test")
	c := openCandidates(ctx, t, e, "contact")[0]

	colleague := e.asOwnScoped(e.otherRep)
	// The precondition the whole test rests on: this seat CAN read the pair.
	// Assert it, because if the read gate refuses first every assertion below
	// passes for the wrong reason.
	if _, err := e.store.GetDedupeCandidate(colleague, c.ID); err != nil {
		t.Fatalf("the colleague must be able to READ the pair, else the write gate is never reached: %v", err)
	}

	if _, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("dismiss without write authority over the pair = %v, want ErrPermissionDenied", err)
	}
	// 403, not 404, and asserted as both halves: GetDedupeCandidate already told
	// this caller the pair is theirs to read, so there is nothing left for
	// existence-hiding to hide. "not ErrNotFound" alone would pass for nil, for
	// a conflict, and for an internal error.
	err := func() error {
		_, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "not_a_duplicate", nil)
		return err
	}()
	if !errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("dismiss refusal = %v, want ErrPermissionDenied and not ErrNotFound", err)
	}

	// The owner still decides their own queue.
	dismissed, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "not_a_duplicate", nil)
	if err != nil {
		t.Fatalf("the owner's own dismiss: %v", err)
	}
	if dismissed.Disposition != "not_a_duplicate" {
		t.Fatalf("owner dismiss left disposition %s, want not_a_duplicate", dismissed.Disposition)
	}

	// Undo is the same write, in reverse: it resurrects a decision the pair's
	// owners made. reopenDedupeCandidate itself stays unprobed on purpose —
	// disposeMerge calls it as the compensating rollback of a failed merge,
	// and a grant revoked mid-flight must not strand a candidate at 'merged'
	// with no merge behind it.
	if _, err := e.store.UndoDedupeDisposition(colleague, c.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("undo without write authority over the pair = %v, want ErrPermissionDenied", err)
	}

	reopened, err := e.store.UndoDedupeDisposition(ctx, c.ID)
	if err != nil {
		t.Fatalf("the owner's own undo: %v", err)
	}
	if reopened.Disposition != "open" {
		t.Fatalf("owner undo left disposition %s, want open", reopened.Disposition)
	}
}

// A pair names TWO records and a verdict changes both, so the probe must hold
// on BOTH ends — the reader it exists for is the caller who may change one side
// and not the other.
//
// Nothing above exercises that. A probe that named the left id twice, or that
// stopped at the first passing arm, refuses the colleague who owns neither
// record exactly as correctly as the real one does, and dismisses the pair for
// a colleague who owns half of it. Run per SIDE for the same reason
