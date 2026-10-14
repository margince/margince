// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asOwnScopeRep is a seat with the seeded rep's forecast grant, the given deal
// grant and own row scope. Every deal is visible to it, and only its own are
// writable.
func (e *scanEnv) asOwnScopeRep(user ids.UUID, deal principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true, Update: true},
				"deal":     deal,
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// openFindingOn mints one open finding about the deal and returns its id.
func (e *scanEnv) openFindingOn(t *testing.T, dealID string) ids.UUID {
	t.Helper()
	finding := Finding{
		Type: TypeClosePast, SubjectID: dealID, Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	var id ids.UUID
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		if err := e.store.UpsertException(ctx, tx, finding, ""); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id FROM assurance_exception WHERE logical_key = $1`,
			LogicalKey(finding)).Scan(&id)
	}); err != nil {
		t.Fatalf("minting the finding: %v", err)
	}
	return id
}

func (e *scanEnv) findingStatus(t *testing.T, id ids.UUID) string {
	t.Helper()
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM assurance_exception WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *scanEnv) seedUser(t *testing.T, label string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`,
		id, label+"-"+id.String()+"@assurance.test", label); err != nil {
		t.Fatal(err)
	}
	return id
}

// A rep answers the findings on deals they may change, and no others.
//
// Every seat reads every deal, so a finding id on a colleague's deal is on the
// list each rep is shown. Admitted, a "not relevant" answer hides the finding
// from that colleague and their lead for up to ninety days.
func TestARepCannotAnswerAFindingOnAColleaguesDeal(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	rep, colleague := e.seedUser(t, "rep"), e.seedUser(t, "colleague")
	theirs := e.openFindingOn(t, e.seedDealOwnedBy(t, colleague))
	mine := e.openFindingOn(t, e.seedDealOwnedBy(t, rep))

	// Resolve judges an answer against the clock it is handed, never the wall.
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	expires := now.AddDate(0, 0, 30)
	hide := Resolution{Outcome: OutcomeNotRelevant, Reason: "checked", ExpiresAt: &expires}
	ctx := e.asOwnScopeRep(rep, principal.ObjectGrant{Read: true, Update: true})

	err := e.store.Resolve(ctx, theirs, hide, now)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("answering a colleague's finding: err = %v, want ErrPermissionDenied", err)
	}
	if got := e.findingStatus(t, theirs); got != "open" {
		t.Errorf("the refused answer still moved the finding to %q", got)
	}

	if err := e.store.Resolve(ctx, mine, hide, now); err != nil {
		t.Fatalf("answering a finding on the rep's own deal: %v", err)
	}
	if got := e.findingStatus(t, mine); got != "resolved" {
		t.Errorf("the rep's own answer left the finding %q, want resolved", got)
	}
}

// A seat whose role may read deals but not change them answers no finding. That
// holds even on a deal it owns, because the answer speaks for the deal.
func TestARoleWithoutDealUpdateCannotAnswerAFinding(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	rep := e.seedUser(t, "rep")
	mine := e.openFindingOn(t, e.seedDealOwnedBy(t, rep))

	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	expires := now.AddDate(0, 0, 30)
	hide := Resolution{Outcome: OutcomeNotRelevant, Reason: "checked", ExpiresAt: &expires}

	err := e.store.Resolve(e.asOwnScopeRep(rep, principal.ObjectGrant{Read: true}), mine, hide, now)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("answering without the deal update grant: err = %v, want ErrPermissionDenied", err)
	}
	if got := e.findingStatus(t, mine); got != "open" {
		t.Errorf("the refused answer still moved the finding to %q", got)
	}
}

// A finding already answered, or never there, answers 404. A different answer
// would tell the caller the finding exists, about a deal they may not open.
func TestAnAnsweredFindingCannotBeAnsweredAgain(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	rep := e.seedUser(t, "rep")
	mine := e.openFindingOn(t, e.seedDealOwnedBy(t, rep))
	ctx := e.asOwnScopeRep(rep, principal.ObjectGrant{Read: true, Update: true})

	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	fixed := Resolution{Outcome: OutcomeFixedRecord}
	if err := e.store.Resolve(ctx, mine, fixed, now); err != nil {
		t.Fatalf("the first answer: %v", err)
	}
	for name, id := range map[string]ids.UUID{"answered": mine, "missing": ids.NewV7()} {
		if err := e.store.Resolve(ctx, id, fixed, now); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s finding: err = %v, want ErrNotFound", name, err)
		}
	}
}

// A finding about anything but a deal has no write authority to ask, so even
// a seat with unbounded scope cannot answer it.
func TestAFindingAboutANonDealSubjectIsNobodysToAnswer(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO assurance_exception
		    (id, logical_key, type, subject_kind, subject_id, severity, captured_by)
		VALUES ($1, $2, $3, 'offer', $4, 'high', 'test')`,
		id, "offer:"+id.String(), TypeClosePast, ids.NewV7()); err != nil {
		t.Fatalf("seeding an offer finding: %v", err)
	}

	err := e.store.Resolve(e.as(), id, Resolution{Outcome: OutcomeFixedRecord},
		time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("answering an offer finding: err = %v, want ErrNotFound", err)
	}
	if got := e.findingStatus(t, id); got != "open" {
		t.Errorf("the refused answer moved the finding to %q", got)
	}
}
