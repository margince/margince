// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One colleague's open domain questions, over a real Postgres.
//
// The read is small and its WHERE carries the whole feature: a question belongs
// to the mailbox owner it was opened for, and it leaves that owner's queue when
// they answer it — including when the answer was "not for me", which is recorded
// as a capture exclusion rather than as a verdict about what the domain IS.
//
// That last part is why these tests exist over a database rather than as a unit
// test with a fake. The answer and the question live in two tables, and nothing
// but SQL joins them.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// excludeDomain writes a capture exclusion the way the capture module's store
// writes one, by hand.
//
// BY HAND, and the reason matters: capture_exclusion is the capture module's
// table and contacts may not import a sibling module to reach its writer. What
// is under test here is the READ's exclusion clause, not the writer — so the row
// is seeded in the shape the writer produces (folded value, scope/user_id
// paired) and the test asserts on what the read does with it.
func (e *dedupeEnv) excludeDomain(ctx context.Context, t *testing.T, scope, value string, user *ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_exclusion (scope, user_id, kind, value, created_by)
			VALUES ($1, $2, 'domain', $3, 'human:test')`, scope, user, value)
		return err
	}); err != nil {
		t.Fatalf("seeding a %s exclusion for %s: %v", scope, value, err)
	}
}

// openQuestionDomains is the reader's own backlog, as domains.
func (e *dedupeEnv) openQuestionDomains(ctx context.Context, t *testing.T) []string {
	t.Helper()
	rows, err := e.store.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatalf("reading open domain questions: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Domain)
	}
	return out
}

// TestAnOpenQuestionReachesTheOwnerWhoseMailRaisedIt is the lane's premise.
//
// The disposition records the mailbox owner at the moment the question opens,
// and this read is the only thing that turns that column into somebody's
// backlog. Without it the fourteen questions a real import produced sat on an
// admin screen addressed to nobody.
func TestAnOpenQuestionReachesTheOwnerWhoseMailRaisedIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "anna@unjudged.test", "Anna Example", "unjudged.test")
	// The crawl found nothing that named a company, which is what leaves the
	// question open with a reason rather than merely queued for a retry.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return markDispositionUnevidenced(ctx, tx, "unjudged.test", "the site named nobody")
	}); err != nil {
		t.Fatalf("withholding the verdict: %v", err)
	}
	open := e.openQuestionDomains(ctx, t)
	if !contains(open, "unjudged.test") {
		t.Fatalf("the owner's backlog = %v, want it to carry unjudged.test", open)
	}
	if reason := e.reasonFor(ctx, t, "unjudged.test"); reason == "" {
		t.Error("the question carries no reason; a row that cannot say why it is open " +
			"asks the reader to decide with nothing to decide from")
	}
}

// reasonFor is the prose the reader is shown for one open question.
func (e *dedupeEnv) reasonFor(ctx context.Context, t *testing.T, domain string) string {
	t.Helper()
	rows, err := e.store.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatalf("reading open domain questions: %v", err)
	}
	for _, row := range rows {
		if row.Domain == domain {
			return row.Reason
		}
	}
	return ""
}

// TestDiscardingADomainTakesItsQuestionOffTheQueue is the discard verb's whole
// contract, and it failed review without this test.
//
// "Not for me" is answered by writing a capture exclusion, which says nothing
// about what the domain IS — so the disposition row stays pending for ever. A
// read keyed only on `pending_reason` therefore serves the same question back on
// the next refresh, and the verb looks like a button that does nothing.
func TestDiscardingADomainTakesItsQuestionOffTheQueue(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "bob@noise.test", "Bob Example", "noise.test")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return markDispositionUnevidenced(ctx, tx, "noise.test", "the site named nobody")
	}); err != nil {
		t.Fatalf("withholding the verdict: %v", err)
	}
	if open := e.openQuestionDomains(ctx, t); !contains(open, "noise.test") {
		t.Fatalf("the backlog = %v, want noise.test before it is discarded", open)
	}

	rep := e.rep
	e.excludeDomain(ctx, t, "user", "noise.test", &rep)

	if open := e.openQuestionDomains(ctx, t); contains(open, "noise.test") {
		t.Errorf("the backlog = %v, and noise.test is still on it after this reader "+
			"discarded it: the question returns on the next refresh and the verb "+
			"reads as a control that does nothing", open)
	}
}

// TestAColleaguesDiscardLeavesYourOwnQuestionStanding is the point of the whole
// split, asserted rather than assumed.
//
// Two colleagues on one installation may judge one domain differently — a
// consultancy that is noise to one seat and a live account to another — and the
// exclusion each writes binds only the connections they granted. A read that
// forgot `user_id` would let one colleague silently answer for everybody, which
// is the shared pile this feature exists to replace.
func TestAColleaguesDiscardLeavesYourOwnQuestionStanding(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "dana@shared.test", "Dana Example", "shared.test")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return markDispositionUnevidenced(ctx, tx, "shared.test", "the site named nobody")
	}); err != nil {
		t.Fatalf("withholding the verdict: %v", err)
	}

	colleague := e.otherRep
	e.excludeDomain(ctx, t, "user", "shared.test", &colleague)

	if open := e.openQuestionDomains(ctx, t); !contains(open, "shared.test") {
		t.Errorf("the backlog = %v, and shared.test left it because a COLLEAGUE "+
			"discarded the domain: one seat answered for another, which is the "+
			"shared pile this queue replaced", open)
	}
}

// TestAWorkspaceExclusionAnswersItForEveryone is the other side of that rule.
//
// A workspace rule is the installation's policy and binds every connection, so a
// question about a domain the workspace refuses is not one any colleague still
// owes an answer to.
func TestAWorkspaceExclusionAnswersItForEveryone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "erin@policy.test", "Erin Example", "policy.test")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return markDispositionUnevidenced(ctx, tx, "policy.test", "the site named nobody")
	}); err != nil {
		t.Fatalf("withholding the verdict: %v", err)
	}
	e.excludeDomain(ctx, t, "workspace", "policy.test", nil)
	if open := e.openQuestionDomains(ctx, t); contains(open, "policy.test") {
		t.Errorf("the backlog = %v, and policy.test survives a WORKSPACE rule: "+
			"the installation refused the domain and the reader is still being asked", open)
	}
}

// TestACallerWithNoHumanBehindItIsRefusedRatherThanServedNothing holds the
// lane's withheld promise.
//
// The feed names a withheld lane in `lanes_omitted`, which it can only do if the
// store REFUSES. Answering an empty list instead would tell a connector — and
// through the lane, a reader — that the backlog is clear, which is a different
// and false statement.
func TestACallerWithNoHumanBehindItIsRefusedRatherThanServedNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	connector := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"},
			Objects:  map[string]principal.ObjectGrant{"company": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := e.store.OpenDomainQuestionsForOwner(connector); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a connector reading open questions answered %v, want ErrPermissionDenied so "+
			"the lane can name itself withheld rather than reporting an empty backlog", err)
	}
}
