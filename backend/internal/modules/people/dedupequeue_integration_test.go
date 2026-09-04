// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The dedupe review queue store (DH-DDL-1, DH-EXT-1/2) over a real
// Postgres: a manual fuzzy create leaves an OPEN candidate the queue
// lists; dismissal suppresses and undoes; merge runs the ONE merge verb
// and cannot be undone; input the queue refuses stays a typed 422; and
// row scope hides a pair whose side the caller cannot see.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedPersonPair creates an incumbent and a fuzzy near-duplicate through
// the real manual-create path — the create itself records the open
// candidate (the PR-12a fold-in under test).
func seedPersonPair(ctx context.Context, t *testing.T, e *dedupeEnv, incumbentName, incumbentEmail, dupName, dupEmail, domain string) (incumbent ids.UUID, created ids.UUID) {
	t.Helper()
	inc, _ := e.seedEmployedPerson(ctx, t, incumbentName, incumbentEmail, "Org "+incumbentName, domain)
	dup, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: dupName, Source: "manual",
		Emails: []PersonEmailInput{{Email: dupEmail, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("fuzzy create must not block: %v", err)
	}
	return inc.UUID, ids.UUID(dup.Id)
}

func openCandidates(ctx context.Context, t *testing.T, e *dedupeEnv, entityType string) []DedupeCandidateRow {
	t.Helper()
	rows, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityType})
	if err != nil {
		t.Fatalf("ListDedupeCandidates: %v", err)
	}
	return rows
}

func TestManualFuzzyCreateLeavesAnOpenDedupeCandidate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, created := seedPersonPair(ctx, t, e, "John Doe", "john@queue.test", "Jon Doe", "jon@queue.test", "queue.test")

	rows := openCandidates(ctx, t, e, "person")
	if len(rows) != 1 {
		t.Fatalf("open queue holds %d candidates, want 1", len(rows))
	}
	c := rows[0]
	if c.Disposition != "open" {
		t.Fatalf("disposition = %s, want open", c.Disposition)
	}
	// Canonical pair: lower id left, whichever side that is.
	got := map[string]bool{c.LeftID.String(): true, c.RightID.String(): true}
	if !got[incumbent.String()] || !got[created.String()] {
		t.Fatalf("pair {%s,%s} does not name incumbent %s + created %s", c.LeftID, c.RightID, incumbent, created)
	}
	if c.Confidence < dedupeReviewThreshold {
		t.Fatalf("confidence %.4f below the review threshold", c.Confidence)
	}
	// The detection-time snapshot names both sides — the queue renders it
	// verbatim, so it must carry the colliding names.
	if ev := string(c.Evidence); !strings.Contains(ev, "Jon Doe") || !strings.Contains(ev, "John Doe") {
		t.Fatalf("evidence %s does not carry both names", ev)
	}

	one, err := e.store.GetDedupeCandidate(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetDedupeCandidate: %v", err)
	}
	if one.ID != c.ID || one.EntityType != "person" {
		t.Fatalf("get returned %s/%s, want %s/person", one.ID, one.EntityType, c.ID)
	}

	if _, err := e.store.GetDedupeCandidate(ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unknown candidate = %v, want ErrNotFound", err)
	}
}

func TestDedupeQueueRefusesBadInput(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Erin Example", "erin@badinput.test", "Eryn Example", "eryn@badinput.test", "badinput.test")
	c := openCandidates(ctx, t, e, "person")[0]

	var input *DedupeInputError
	if _, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "bogus", nil); !errors.As(err, &input) {
		t.Fatalf("bogus disposition = %v, want DedupeInputError", err)
	}
	if _, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "merge", nil); !errors.As(err, &input) {
		t.Fatalf("merge without winner = %v, want DedupeInputError", err)
	}
	stranger := ids.NewV7()
	if _, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "merge", &stranger); !errors.As(err, &input) {
		t.Fatalf("winner outside the pair = %v, want DedupeInputError", err)
	}
	// A malformed page token is the SAME refusal every paginated endpoint gives,
	// so the queue answers the contract's malformed_cursor rather than its own
	// module-shaped "invalid".
	var badCursor *storekit.MalformedCursorError
	for _, token := range []string{
		"not-base64!",
		// Valid base64 of valid JSON that is not a cursor. `null` and `{}`
		// unmarshal without error and leave the keyset at its zero value, which
		// would read as a real position and page from the top of the queue
		// instead of refusing.
		"bnVsbA",  // null
		"e30",     // {}
		"WzEsMl0", // [1,2]
	} {
		if _, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{Cursor: token}); !errors.As(err, &badCursor) {
			t.Errorf("cursor %q = %v, want MalformedCursorError", token, err)
		}
	}
}

func TestDedupeDismissSuppressesAndUndoReopens(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Max Muster", "max@dismiss.test", "Marx Muster", "marx@dismiss.test", "dismiss.test")
	c := openCandidates(ctx, t, e, "person")[0]

	dismissed, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "not_a_duplicate", nil)
	if err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if dismissed.Disposition != "not_a_duplicate" || dismissed.DisposedBy == nil || dismissed.DisposedAt == nil {
		t.Fatalf("dismissed row = %+v, want not_a_duplicate with disposer + timestamp", dismissed)
	}
	if rows := openCandidates(ctx, t, e, "person"); len(rows) != 0 {
		t.Fatalf("dismissed pair still lists open (%d rows)", len(rows))
	}
	byStatus, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{Status: "not_a_duplicate"})
	if err != nil || len(byStatus) != 1 {
		t.Fatalf("status filter found %d rows (err %v), want the dismissed pair", len(byStatus), err)
	}

	// A decided pair refuses a second decision — conflict, never a
	// silent double-merge.
	if _, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second dispose = %v, want ErrConflict", err)
	}

	reopened, err := e.store.UndoDedupeDisposition(ctx, c.ID)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if reopened.Disposition != "open" || reopened.DisposedBy != nil {
		t.Fatalf("reopened row = %+v, want open with no disposer", reopened)
	}
	// Undoing an already-open pair is a conflict, not a no-op success.
	if _, err := e.store.UndoDedupeDisposition(ctx, c.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo on open = %v, want ErrConflict", err)
	}
}

func TestDedupeMergeRunsTheOneMergeVerbAndStandsForever(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, created := seedPersonPair(ctx, t, e, "Ada Lovelace", "ada@merge.test", "Ada Lovelance", "adal@merge.test", "merge.test")
	c := openCandidates(ctx, t, e, "person")[0]

	winner := incumbent
	merged, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "merge", &winner)
	if err != nil {
		t.Fatalf("merge dispose: %v", err)
	}
	if merged.Disposition != "merged" {
		t.Fatalf("disposition = %s, want merged", merged.Disposition)
	}
	// The loser carries merged_into_id — the merge verb really ran.
	var mergedInto *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT merged_into_id FROM person WHERE id = $1`, created).Scan(&mergedInto)
	}); err != nil {
		t.Fatalf("reading loser: %v", err)
	}
	if mergedInto == nil || *mergedInto != winner {
		t.Fatalf("loser merged_into_id = %v, want %s", mergedInto, winner)
	}
	// PO-AC-M6: merge reversal does not exist — the queue must not
	// pretend otherwise.
	if _, err := e.store.UndoDedupeDisposition(ctx, c.ID); !errors.Is(err, ErrNotUndoable) {
		t.Fatalf("undo on merged = %v, want ErrNotUndoable", err)
	}
}

func TestDedupeOrgMergeArm(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Globex Corporation GmbH", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "globex.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Globex Corporation Inc", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "globex-us.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("fuzzy org create must not block: %v", err)
	}

	rows := openCandidates(ctx, t, e, "organization")
	if len(rows) != 1 {
		t.Fatalf("open org queue holds %d candidates, want 1", len(rows))
	}
	winner := ids.UUID(incumbent.Id)
	merged, err := e.store.DisposeDedupeCandidate(ctx, rows[0].ID, "merge", &winner)
	if err != nil {
		t.Fatalf("org merge dispose: %v", err)
	}
	if merged.Disposition != "merged" {
		t.Fatalf("disposition = %s, want merged", merged.Disposition)
	}
}

func TestDedupeQueuePagesByConfidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Kim Page", "kim@page.test", "Kym Page", "kym@page.test", "page.test")
	_, _ = seedPersonPair(ctx, t, e, "Lee Cursor", "lee@cursor.test", "Leigh Cursor", "leigh@cursor.test", "cursor.test")

	first, next, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{Limit: 1})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first) != 1 || next == "" {
		t.Fatalf("page 1 = %d rows, cursor %q — want 1 row and a cursor", len(first), next)
	}
	second, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{Limit: 1, Cursor: next})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(second) != 1 || second[0].ID == first[0].ID {
		t.Fatalf("page 2 re-served the first row")
	}
	// Confidence-descending: the keyset order is the queue's contract.
	if second[0].Confidence > first[0].Confidence {
		t.Fatalf("page order broken: %.4f after %.4f", second[0].Confidence, first[0].Confidence)
	}
}

// asAgent is a non-human principal — the disposition verbs are human-only
// whatever the transport claims.
func (e *dedupeEnv) asAgent() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.rep,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"person":       {Create: true, Read: true, Update: true},
				"organization": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestDedupeDispositionIsHumanOnly(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Pat Human", "pat@human.test", "Patt Human", "patt@human.test", "human.test")
	c := openCandidates(ctx, t, e, "person")[0]

	if _, err := e.store.DisposeDedupeCandidate(e.asAgent(), c.ID, "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent dispose = %v, want ErrPermissionDenied", err)
	}
	if _, err := e.store.UndoDedupeDisposition(e.asAgent(), c.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent undo = %v, want ErrPermissionDenied", err)
	}
}

// asOwnScoped is a bounded human seat: own-only row scope, and {Read, Update}
// on all three dedupe entity types. What it can actually reach depends on the
// rows a test leaves behind, and the two states pull in opposite directions —
// a capture-private pair is hidden from it by the READ gate, while a pair at
// the default visibility='workspace' it can read but, owning neither record,
// still may not decide. Each test states which it is seeding.
func (e *dedupeEnv) asOwnScoped(other ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + other.String(), UserID: other,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"person":       {Read: true, Update: true},
				"organization": {Read: true, Update: true},
				"lead":         {Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

func TestDedupeQueueHidesPairsOutsideTheCallersRowScope(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Vis Owner", "vis@scope.test", "Viz Owner", "viz@scope.test", "scope.test")
	c := openCandidates(ctx, t, e, "person")[0]

	// A person is workspace-readable identity, so ownership alone hides
	// nothing: the pair's people become e.rep's capture-private contacts
	// (visibility='owner'), the one state that still narrows a person read.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE person SET owner_id = $1, visibility = 'owner'`, e.rep)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	other := e.asOwnScoped(ids.NewV7())
	if rows := openCandidates(other, t, e, "person"); len(rows) != 0 {
		t.Fatalf("own-scoped stranger sees %d candidates, want 0", len(rows))
	}
	if _, err := e.store.GetDedupeCandidate(other, c.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("out-of-scope get = %v, want ErrNotFound (existence-hiding)", err)
	}
	// The owner still sees the pair.
	if rows := openCandidates(ctx, t, e, "person"); len(rows) != 1 {
		t.Fatalf("owner sees %d candidates, want 1", len(rows))
	}
}

// seedOrgPair leaves one open organization candidate: two spellings of one
// company, no shared exact key.
func seedOrgPair(ctx context.Context, t *testing.T, e *dedupeEnv) (ids.UUID, ids.UUID) {
	t.Helper()
	first, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Globex Corporation GmbH", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "globex.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed incumbent org: %v", err)
	}
	second, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Globex Corporation Inc", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "globex-us.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed near-duplicate org: %v", err)
	}
	return ids.UUID(first.Id), ids.UUID(second.Id)
}

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
// Run per entity type because the clause spells person and organization
// separately, and per SIDE because it spells left and right separately too.
// A lead has no arm here: it is workspace-readable identity with no capture
// privacy, so no human seat can ever see only half of a lead pair.
func TestDedupeQueueHidesAPairTheCallerCanOnlyHalfSee(t *testing.T) {
	arms := []halfVisibleArm{
		{
			entityPerson,
			`UPDATE person SET owner_id = $1, visibility = 'owner' WHERE id = $2`,
			`UPDATE person SET owner_id = $1, visibility = 'workspace' WHERE id = $2`,
			func(ctx context.Context, t *testing.T, e *dedupeEnv) (ids.UUID, ids.UUID) {
				return seedPersonPair(ctx, t, e, "John Doe", "john@queue.test", "Jon Doe", "jon@queue.test", "queue.test")
			},
		},
		{
			entityOrganization,
			`UPDATE organization SET owner_id = $1, visibility = 'owner' WHERE id = $2`,
			`UPDATE organization SET owner_id = $1, visibility = 'workspace' WHERE id = $2`,
			seedOrgPair,
		},
	}
	for _, arm := range arms {
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
// mergePair. The object grant is not that authority: person and organization
// are workspace-readable identity, so every seat holding person:update passes
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
	_, _ = seedPersonPair(ctx, t, e, "Ada Byron", "ada@authority.test", "Adah Byron", "adah@authority.test", "authority.test")
	c := openCandidates(ctx, t, e, "person")[0]

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
// TestDedupeQueueHidesAPairTheCallerCanOnlyHalfSee does: the pair is stored
// canonically with the lower id left and ids are time-ordered, so handing over
// the record created first always probes the left slot and never the right.
func TestDedupeDispositionRefusesAHalfWritablePair(t *testing.T) {
	// Both verbs, because they are separate probes on separate paths: an undo
	// that checked only one endpoint would pass the neither-writable test and
	// the both-writable test alike. Both entity types that can reach this
	// state, because entityType is threaded from the row and a helper wired to
	// person alone would look identical from the person arm. Lead needs no arm:
	// its pairs are seeded through the same path and the threading is already
	// covered.
	arms := []struct {
		entityType string
		reown      string
		seed       func(context.Context, *testing.T, *dedupeEnv) (ids.UUID, ids.UUID)
	}{
		{
			entityPerson, `UPDATE person SET owner_id = $1 WHERE id = $2`,
			func(ctx context.Context, t *testing.T, e *dedupeEnv) (ids.UUID, ids.UUID) {
				return seedPersonPair(ctx, t, e, "Ola Half", "ola@half.test", "Olah Half", "olah@half.test", "half.test")
			},
		},
		{entityOrganization, `UPDATE organization SET owner_id = $1 WHERE id = $2`, seedOrgPair},
	}
	for _, arm := range arms {
		for _, give := range []string{"first-created", "second-created"} {
			for _, verb := range []string{"dismiss", "undo"} {
				t.Run(arm.entityType+"/"+give+"/"+verb, func(t *testing.T) {
					halfWritablePairIsRefused(t, arm.entityType, arm.reown, arm.seed, give, verb)
				})
			}
		}
	}
}

func halfWritablePairIsRefused(t *testing.T, entityType, reown string,
	seed func(context.Context, *testing.T, *dedupeEnv) (ids.UUID, ids.UUID), give, verb string,
) {
	t.Helper()
	e := setupDedupe(t)
	ctx := e.as()
	first, second := seed(ctx, t, e)
	c := openCandidates(ctx, t, e, entityType)[0]

	owned := first
	if give == "second-created" {
		owned = second
	}
	// Through an UPDATE rather than a second create: the point is a pair whose
	// sides have different owners, and the manual create path stamps the acting
	// principal on both.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, reown, e.otherRep, owned)
		return err
	}); err != nil {
		t.Fatalf("handing one side to the colleague: %v", err)
	}

	colleague := e.asOwnScoped(e.otherRep)
	if _, err := e.store.GetDedupeCandidate(colleague, c.ID); err != nil {
		t.Fatalf("the colleague must still READ the pair: %v", err)
	}

	if verb == "undo" {
		// Something to undo, disposed by the seat that holds both ends.
		if _, err := e.store.DisposeDedupeCandidate(ctx, c.ID, "not_a_duplicate", nil); err != nil {
			t.Fatalf("seeding the disposition to undo: %v", err)
		}
		if _, err := e.store.UndoDedupeDisposition(colleague, c.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("undo holding write authority over only %s = %v, want ErrPermissionDenied", give, err)
		}
		return
	}
	if _, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("dismiss holding write authority over only %s = %v, want ErrPermissionDenied", give, err)
	}
}

// The admission direction, which nothing else here covers. Every other happy
// path in this package acts as e.as() — RowScope: RowScopeAll — and
// auth.Unbounded short-circuits ensureWriteAuthority before a single row is
// read, so the owner arm of writeAuthorityPredicate never executes on those
// runs. An ensurePairWritable that refused EVERY bounded principal (wrong
// alias, inverted predicate, a liveness filter the pair cannot satisfy) would
// ship with the refusal tests and the whole suite green.
//
// So this seat is bounded — own-only scope, no grant — and owns both records
// because the manual create path stamped it. It must be admitted.
func TestDedupeDispositionAdmitsABoundedOwnerOfBothRecords(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, _ = seedPersonPair(ctx, t, e, "Rey Owner", "rey@admit.test", "Reye Owner", "reye@admit.test", "admit.test")
	c := openCandidates(ctx, t, e, "person")[0]

	// Same user as the seeding principal, but bounded: RowScopeOwn, so the
	// probe renders its predicate and runs it rather than being waved through.
	owner := e.asOwnScoped(e.rep)

	dismissed, err := e.store.DisposeDedupeCandidate(owner, c.ID, "not_a_duplicate", nil)
	if err != nil {
		t.Fatalf("a bounded seat owning BOTH records must be admitted: %v", err)
	}
	if dismissed.Disposition != "not_a_duplicate" {
		t.Fatalf("bounded owner dismiss left disposition %s, want not_a_duplicate", dismissed.Disposition)
	}

	reopened, err := e.store.UndoDedupeDisposition(owner, c.ID)
	if err != nil {
		t.Fatalf("a bounded seat owning BOTH records must be admitted on undo: %v", err)
	}
	if reopened.Disposition != "open" {
		t.Fatalf("bounded owner undo left disposition %s, want open", reopened.Disposition)
	}
}

// The merge arm keeps its own refusal, and this pins it. mergePair treats the
// two ends asymmetrically on purpose: an unwritable SOURCE answers the
// authority error, while an unwritable TARGET answers a BARE conflict rather
// than naming itself, because a merge returns the survivor and the refusal must
// disclose no more than the caller could already read.
//
// A both-ends probe placed before the switch would pre-empt that with 403 and
// silently convert a documented 409 — which is why the dismiss probe lives in
// its own arm. Nothing else in this package would notice: every other merge
// test acts with RowScopeAll, for which auth.Unbounded waves the whole question
// through.
func TestDedupeMergeArmKeepsItsOwnRefusal(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, second := seedPersonPair(ctx, t, e, "Ivy Merge", "ivy@mergearm.test", "Ivee Merge", "ivee@mergearm.test", "mergearm.test")
	c := openCandidates(ctx, t, e, "person")[0]

	// The colleague owns the LOSER and not the winner: writable source,
	// unwritable target, which is mergePair's bare-conflict case.
	winner, loser := first, second
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE person SET owner_id = $1 WHERE id = $2`, e.otherRep, loser)
		return err
	}); err != nil {
		t.Fatalf("handing the loser to the colleague: %v", err)
	}

	colleague := e.asOwnScoped(e.otherRep)
	err := func() error {
		_, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "merge", &winner)
		return err
	}()
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merge onto an unwritable winner = %v, want ErrConflict (mergePair's bare conflict, not 403)", err)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatal("the merge arm answered ErrPermissionDenied — a probe outside the not_a_duplicate arm has pre-empted mergePair's disclosure decision")
	}

	// And input validation still precedes authority on this arm: a winner
	// outside the pair is the caller's mistake to hear about.
	stranger := ids.NewV7()
	var input *DedupeInputError
	if _, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "merge", &stranger); !errors.As(err, &input) {
		t.Fatalf("winner outside the pair = %v, want DedupeInputError", err)
	}
}

// OpenCandidatesNaming answers a create's own question — "did I just get filed
// against something?" — and it must answer it under the SAME visibility rule
// the paged queue applies, not a looser one.
//
// The rule is that a pair surfaces only when BOTH sides are visible to the
// caller, because the evidence snapshot quotes both records: naming a pair is a
// read of them. A narrow read that skipped it would let a caller learn that a
// record it may not see exists, and learn its id, by creating something that
// collides with it — a disclosure through a write, which is the shape nobody
// goes looking for.
//
// This also holds the placeholder arithmetic honest. The visibility clause
// numbers its own parameters by appending to the arg slice, and this query
// starts that slice at a different length than the paged one does. An off-by-one
// there binds the wrong value rather than failing, and the symptom is exactly
// the disclosure above.
func TestOpenCandidatesNamingHidesAPairOutsideTheCallersRowScope(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, right := seedPersonPair(ctx, t, e, "Nam Owner", "nam@scope.test", "Namm Owner", "namm@scope.test", "scope.test")

	// Capture-private to e.rep, the one state that still narrows a person read.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE person SET owner_id = $1, visibility = 'owner'`, e.rep)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	stranger, err := e.store.OpenCandidatesNaming(e.asOwnScoped(ids.NewV7()), entityPerson, right)
	if err != nil {
		t.Fatalf("an out-of-scope read must answer empty, not fail: %v", err)
	}
	if len(stranger) != 0 {
		t.Fatalf("an own-scoped stranger is told about %d candidates naming a record it cannot see, want 0", len(stranger))
	}

	// And the owner is still told, or the check above passes for the wrong
	// reason — a query that answers nobody hides nothing.
	owner, err := e.store.OpenCandidatesNaming(ctx, entityPerson, right)
	if err != nil {
		t.Fatalf("the owner's read: %v", err)
	}
	if len(owner) != 1 {
		t.Fatalf("the owner is told about %d candidates, want 1", len(owner))
	}
	if owner[0].LeftID != right && owner[0].RightID != right {
		t.Errorf("the candidate names {%s, %s}, neither of which is the record asked about, %s",
			owner[0].LeftID, owner[0].RightID, right)
	}
}

// A record type with no dedupe queue is entitled to ask and be told nothing,
// rather than to fail. create_record serves seven record types and only three
// have a queue at all, so a refusal here would turn every deal create into an
// error.
func TestOpenCandidatesNamingIsSilentForARecordTypeWithNoQueue(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	rows, err := e.store.OpenCandidatesNaming(ctx, "deal", ids.NewV7())
	if err != nil {
		t.Fatalf("a record type with no queue must answer empty, not fail: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d candidates for a type that has no queue, want 0", len(rows))
	}
}

// What the CARD offers and what the ENDPOINT accepts have to be one answer.
//
// The Worklist decides whether to draw the verbs by asking DecidableForMerge;
// the disposition endpoint decides whether to accept the press by asking
// ensurePairWritable. They are separate paths, so this pins them against the
// same fixtures: a seat owning both records is offered the verbs and admitted,
// and a seat owning neither is offered nothing and refused.
//
// A unit test cannot hold this. Both answers come out of SQL predicates over
// owner_id and record_grant, which is exactly what a stub replaces.
func TestTheMergeCardAgreesWithTheDispositionEndpoint(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left, right := seedOrgPair(ctx, t, e)
	c := openCandidates(ctx, t, e, "organization")[0]

	// The owner of BOTH records: the endpoint admits this seat, so the card
	// must offer it the verbs.
	owner := e.asOwnScoped(e.rep)
	decidable, err := e.store.DecidableForMerge(owner, entityOrganization, []ids.UUID{left, right})
	if err != nil {
		t.Fatalf("asking what the owner may decide: %v", err)
	}
	if !decidable[left] || !decidable[right] {
		t.Fatalf("the card withheld the verbs from a seat owning both records (left=%v right=%v), "+
			"and the endpoint would have accepted its press", decidable[left], decidable[right])
	}

	// The colleague owns NEITHER record. They may read the pair — everyone here
	// reads the workspace's customer records — and the endpoint refuses them,
	// so the card must not offer what would refuse.
	colleague := e.asOwnScoped(e.otherRep)
	withheld, err := e.store.DecidableForMerge(colleague, entityOrganization, []ids.UUID{left, right})
	if err != nil {
		t.Fatalf("asking what a colleague may decide: %v", err)
	}
	if withheld[left] || withheld[right] {
		t.Fatalf("the card offered a verb over records the colleague owns neither of "+
			"(left=%v right=%v) — the press this invites is the 403 that told them to try again",
			withheld[left], withheld[right])
	}
	// And the endpoint really does refuse, so the assertion above is pinned to
	// the behaviour rather than to an assumption about it.
	if _, err := e.store.DisposeDedupeCandidate(colleague, c.ID, "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the endpoint admitted a colleague owning neither record (%v), "+
			"so withholding the verb from them is the card disagreeing with the write", err)
	}
}
