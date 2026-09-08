// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Every writer that decides a person's current-primary employment from a read
// of their other employments waits for whoever is already deciding.
//
// Asserted by BLOCKING rather than by racing goroutines. A race reproduces this
// defect about one run in six, which is a test that reports a regression as an
// occasional flake — and the shape it produces is
// `uq_rel_current_primary_employer` refusing a caller that never sent the flag,
// reachable by no ordering of the two writers at all. The caller it reaches
// first is an inbound email nobody is watching.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// holdEmploymentLock opens a transaction holding one person's employment write
// identity and hands it back still open, so the test owns the moment a second
// writer may proceed.
//
// Taken through the production call rather than a copy of its key: a change to
// how the identity is spelled can never leave this transaction holding
// something no writer waits on.
func (e *dedupeEnv) holdEmploymentLock(ctx context.Context, t *testing.T, person ids.PersonID) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the lock holder's transaction: %v", err)
	}
	// A transaction left open on a failure path holds the lock and a pooled
	// connection with it, and the run that meant to fail loudly would hang.
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the lock holder's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the lock holder's backend pid: %v", err)
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, person.String()); err != nil {
		t.Fatalf("taking the employment write identity for %s: %v", person, err)
	}
	return tx, pid
}

// Capture plants an employment for a person it reads as having no primary one.
// A patch granting the flag to another of their employments is the same
// decision from the other side, and holds this lock while it makes it — so
// capture waiting for the lock IS capture refusing to decide from a read
// somebody else is invalidating.
func TestCaptureWaitsOnThePersonsEmploymentLock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person := e.seedUnmarkedIncumbent(ctx, t)
	// The company that owns the person's OWN email domain, so capture has an
	// employment to plant rather than one that already exists.
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "New Employer GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "newemployer-race.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the capture target: %v", err)
	}
	captureInput := e.ensureInput(ctx, t, "subject@newemployer-race.test", "Race Subject", "newemployer-race.test")

	holder, pid := e.holdEmploymentLock(ctx, t, person)
	done := make(chan error, 1)
	go func() {
		_, err := e.store.EnsureCounterparty(e.as(), captureInput)
		done <- err
	}()
	mustBlockOn(t, holder, pid, done)

	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	mustFinishCleanly(t, "capture", done)
	if got := e.currentPrimaryCount(ctx, t, person); got != 1 {
		t.Errorf("%d current primary employments, want exactly 1", got)
	}
}

// The domain-triage backlog is the same decision in bulk: a primary employment
// for every person on a domain who reads as having none. It answered from a
// snapshot taken before it had locked anybody.
func TestTheDomainBacklogWaitsOnEachPersonsEmploymentLock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person := e.seedUnmarkedIncumbent(ctx, t)
	// A second person on the same domain, so this is the bulk path it exists to
	// be rather than a one-row special case.
	e.openTriageFirst(ctx, t, "colleague@newemployer-race.test", "A Colleague", "newemployer-race.test")
	readID := e.startTriageRead(ctx, t, "newemployer-race.test")

	holder, pid := e.holdEmploymentLock(ctx, t, person)
	done := make(chan error, 1)
	go func() {
		_, err := e.store.ResolveDomainTriage(e.as(), ResolveDomainTriageInput{
			Domain: "newemployer-race.test", Status: DomainCompany, Source: DomainSourceSiteRead,
			Evidence: "the site states a legal entity", ReadID: readID,
			DossierName: "New Employer GmbH", SeedURL: "https://newemployer-race.test",
		})
		done <- err
	}()
	mustBlockOn(t, holder, pid, done)

	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	mustFinishCleanly(t, "the domain backlog", done)
	if got := e.currentPrimaryCount(ctx, t, person); got != 1 {
		t.Errorf("%d current primary employments, want exactly 1", got)
	}
}

func mustFinishCleanly(t *testing.T, who string, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s failed once the lock was free: %v — no ordering of these two writers "+
				"refuses either of them", who, err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("%s never finished after the lock was released", who)
	}
}

// asEditor is the same rep the other fixtures use, with the relationship UPDATE
// grant the patch half of these cases needs. The shared context grants create
// and read only, and widening it there would quietly hand every other case in
// this package an authority it was written without.
func (e *dedupeEnv) asEditor() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"person":       {Create: true, Read: true, Update: true},
				"company":      {Create: true, Read: true, Update: true},
				"relationship": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// currentPrimaryCount is asked of the database rather than of any caller's
// return: every reader of the flag — the account's contact count, the person
// list's employer filter, the company People tab — answers wrong for a person
// carrying two.
func (e *dedupeEnv) currentPrimaryCount(ctx context.Context, t *testing.T, person ids.PersonID) int {
	t.Helper()
	// Through the production predicates: a count that spelled the slot by hand
	// would keep passing after the definition moved under it.
	return e.countInWorkspace(ctx, t, `
		SELECT count(*) FROM relationship
		WHERE person_id = $1 AND `+employment.CurrentPrimarySlotSQL("")+`
		  AND `+employment.IsCurrentSQL("ended_at"), person)
}

// seedUnmarkedIncumbent gives one person an employment at a company that is NOT
// their email's domain, with the flag explicitly off — the state in which a
// capture and a patch both have a decision to make about the same person.
func (e *dedupeEnv) seedUnmarkedIncumbent(ctx context.Context, t *testing.T) ids.PersonID {
	t.Helper()
	person, _ := e.seedEmployedPerson(ctx, t,
		"Race Subject", "subject@newemployer-race.test", "Incumbent Employer GmbH", "incumbent-race.test")
	var incumbent ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM relationship WHERE kind = 'employment' AND person_id = $1`, person).Scan(&incumbent)
	}); err != nil {
		t.Fatalf("reading the seeded employment: %v", err)
	}
	no := false
	if _, err := e.store.UpdateRelationship(e.asEditor(), incumbent,
		UpdateRelationshipInput{IsCurrentPrimary: &no}); err != nil {
		t.Fatalf("clearing the incumbent's flag: %v", err)
	}
	return person
}

// The candidate list is everybody on the domain, and that is the point.
//
// Asking who already has a primary employment here would answer the decision
// from an UNLOCKED read: a person whose only employment ends between this read
// and the insert would be missing from the list, and the insert can only
// reconsider the ids it was handed — leaving them employed once and unmarked,
// which is the state this whole lock exists to prevent.
func TestTheDomainCandidateListDoesNotPreJudgeWhoNeedsAnEmployment(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// This person's employment is at another company and IS their primary one,
	// so a list that pre-judged would leave them out.
	settled, _ := e.seedEmployedPerson(ctx, t,
		"Already Employed", "settled@newemployer-race.test", "Incumbent Employer GmbH", "incumbent-race.test")
	// And one with nothing, who is a candidate under either reading.
	e.openTriageFirst(ctx, t, "colleague@newemployer-race.test", "A Colleague", "newemployer-race.test")

	var candidates []ids.PersonID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		candidates, err = domainEmploymentCandidates(ctx, tx, "newemployer-race.test")
		return err
	}); err != nil {
		t.Fatalf("reading the candidates: %v", err)
	}

	found := false
	for _, one := range candidates {
		if one == settled {
			found = true
		}
	}
	if !found {
		t.Errorf("the person who already has a primary employment is not in the candidate list, so "+
			"the insert could never reconsider them: %v", candidates)
	}
	if len(candidates) < 2 {
		t.Errorf("%d candidate(s) on the domain, want both people — a list this short is one that "+
			"answered the decision instead of naming who it is about", len(candidates))
	}
}
