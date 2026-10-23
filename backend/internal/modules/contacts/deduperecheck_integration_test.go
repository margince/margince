// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The open duplicate queue re-checked against today's filing rule. A pair an
// older rule filed is archived with an audit row. A pair the rule still files
// stays, and so does a pair someone decided. Filing a closed pair again
// reopens the same row.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asRecheck is the context the nightly job runs the pass under.
func (e *dedupeEnv) asRecheck() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.SystemActing(ctx, "dedupe_recheck")
}

func (e *dedupeEnv) plainContact(ctx context.Context, t *testing.T, name, email string) ids.UUID {
	t.Helper()
	created, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

// fileOldRulePair files a pair through the real writer at the score an older
// rule gave it. That is how a queue built before a stricter rule looks.
func (e *dedupeEnv) fileOldRulePair(ctx context.Context, t *testing.T, entityType string, a, b ids.UUID, confidence float64) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := recordDedupeCandidate(ctx, tx, entityType, a, b, confidence,
			[]map[string]any{{evidenceFieldKey: fieldFullName}}, "manual", "human:"+e.rep.String()); err != nil {
			return err
		}
		left, right := a, b
		if right.String() < left.String() {
			left, right = right, left
		}
		return tx.QueryRow(ctx, `
			SELECT id FROM dedupe_candidate
			 WHERE coalesce(left_contact_id, left_company_id, left_lead_id) = $1
			   AND coalesce(right_contact_id, right_company_id, right_lead_id) = $2`, left, right).Scan(&id)
	})
	if err != nil {
		t.Fatalf("file %s pair: %v", entityType, err)
	}
	return id
}

// candidateState is a pair's archived flag and the audit verbs on its history.
func (e *dedupeEnv) candidateState(ctx context.Context, t *testing.T, id ids.UUID) (archived bool, verbs []string) {
	t.Helper()
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT archived_at IS NOT NULL FROM dedupe_candidate WHERE id = $1`, id).Scan(&archived); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT action FROM audit_log WHERE entity_type = 'dedupe_candidate' AND entity_id = $1
			 ORDER BY occurred_at, id`, id)
		if err != nil {
			return err
		}
		verbs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("read candidate %s: %v", id, err)
	}
	return archived, verbs
}

func (e *dedupeEnv) recheck(t *testing.T) DedupeRecheckResult {
	t.Helper()
	result, err := e.store.RecheckOpenDedupeCandidates(e.asRecheck())
	if err != nil {
		t.Fatalf("RecheckOpenDedupeCandidates: %v", err)
	}
	return result
}

func TestTheRecheckClosesAContactPairTheRuleNoLongerFilesAndKeepsTheRest(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	stale := e.fileOldRulePair(ctx, t, entityContact,
		e.plainContact(ctx, t, "Alex Johnson", "alex.johnson@recheck.test"),
		e.plainContact(ctx, t, "Alex Meyer", "alex.meyer@recheck.test"), 0.88)
	corroborated := e.fileOldRulePair(ctx, t, entityContact,
		e.plainContact(ctx, t, "Jonathan Weber", "jonathan@recheck.test"),
		e.plainContact(ctx, t, "Jonathon Weber", "jonathon@recheck.test"), 0.9)
	sameName := e.fileOldRulePair(ctx, t, entityContact,
		e.plainContact(ctx, t, "Mia Schulz", "mia@one.example"),
		e.plainContact(ctx, t, "Mia Schulz", "mia@two.example"), 0)
	sharedPhone := e.fileOldRulePair(ctx, t, entityContact,
		e.plainContact(ctx, t, "Ola Berg", "ola@phone.example"),
		e.plainContact(ctx, t, "Kim Lund", "kim@phone.example"), identityConflictConfidence)

	if got := e.recheck(t); got.Closed != 1 {
		t.Fatalf("the pass closed %d pairs, want 1 (%+v)", got.Closed, got)
	}
	if archived, verbs := e.candidateState(ctx, t, stale); !archived || verbs[len(verbs)-1] != "archive" {
		t.Errorf("the old-rule pair: archived=%v history=%v, want archived with an archive audit row", archived, verbs)
	}
	for name, id := range map[string]ids.UUID{
		"corroborated": corroborated, "same name": sameName, "shared phone": sharedPhone,
	} {
		if archived, _ := e.candidateState(ctx, t, id); archived {
			t.Errorf("the %s pair was closed; today's rule still files it", name)
		}
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Errorf("a second pass closed %d more pairs; it must find nothing to do", got.Closed)
	}
}

func TestTheRecheckLeavesAPairSomeoneDecided(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.fileOldRulePair(ctx, t, entityContact,
		e.plainContact(ctx, t, "Alex Johnson", "alex.johnson@decided.test"),
		e.plainContact(ctx, t, "Alex Meyer", "alex.meyer@decided.test"), 0.88)
	if _, err := e.store.DisposeDedupeCandidate(ctx, id, "not_a_duplicate", nil); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if _, err := e.store.UndoDedupeDisposition(ctx, id); err != nil {
		t.Fatalf("undo: %v", err)
	}
	e.recheck(t)
	if archived, _ := e.candidateState(ctx, t, id); archived {
		t.Error("a pair someone dismissed and reopened was closed by the system")
	}
}

func TestFilingAClosedPairAgainReopensTheSameRow(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.plainContact(ctx, t, "Alex Johnson", "alex.johnson@refile.test")
	right := e.plainContact(ctx, t, "Alex Meyer", "alex.meyer@refile.test")
	id := e.fileOldRulePair(ctx, t, entityContact, left, right, 0.88)
	e.recheck(t)

	if again := e.fileOldRulePair(ctx, t, entityContact, left, right, 0.91); again != id {
		t.Fatalf("filing again made row %s, want the archived row %s back", again, id)
	}
	archived, verbs := e.candidateState(ctx, t, id)
	if archived || verbs[len(verbs)-1] != "restore" {
		t.Errorf("after filing again: archived=%v history=%v, want open with a restore audit row", archived, verbs)
	}
}

func (e *dedupeEnv) plainLead(ctx context.Context, t *testing.T, name, email string) ids.UUID {
	t.Helper()
	created, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Email: &email, Source: "manual"})
	if err != nil {
		t.Fatalf("create lead %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

func (e *dedupeEnv) plainCompany(ctx context.Context, t *testing.T, name string) ids.UUID {
	t.Helper()
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("create company %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

func TestTheRecheckClosesLeadAndCompanyPairsTheRuleNoLongerFiles(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := func(name, email string) ids.UUID { return e.plainLead(ctx, t, name, email) }
	company := func(name string) ids.UUID { return e.plainCompany(ctx, t, name) }
	staleLead := e.fileOldRulePair(ctx, t, entityLead,
		lead("Alex Johnson", "alex.johnson@leads.test"), lead("Alex Meyer", "alex.meyer@leads.test"), 0.88)
	consumerMailLead := e.fileOldRulePair(ctx, t, entityLead,
		lead("Alex Johnson", "alex.johnson@gmail.com"), lead("Alex Meyer", "alex.meyer@gmail.com"), 0.88)
	// Filed at 1.0 when both leads named one company; now they share only a name.
	sameNameLead := e.fileOldRulePair(ctx, t, entityLead,
		lead("Mia Schulz", "mia@first.example"), lead("Mia Schulz", "mia@second.example"), 1)
	staleCompany := e.fileOldRulePair(ctx, t, entityCompany, company("Northwind Traders"), company("Contoso Pharma"), 0.8)
	keptCompany := e.fileOldRulePair(ctx, t, entityCompany, company("Fabrikam GmbH"), company("Fabrikam AG"), 0.9)

	e.recheck(t)
	for name, tc := range map[string]struct {
		id     ids.UUID
		closed bool
	}{
		"lead pair with different family names": {staleLead, true},
		"lead pair sharing only a mail host":    {consumerMailLead, true},
		"lead pair sharing only a name":         {sameNameLead, true},
		"company pair sharing no word":          {staleCompany, true},
		"company pair with one brand":           {keptCompany, false},
	} {
		if archived, _ := e.candidateState(ctx, t, tc.id); archived != tc.closed {
			t.Errorf("%s: archived=%v, want %v", name, archived, tc.closed)
		}
	}
}

// An end archived after filing cannot be scored, so the pass leaves the pair for
// a reviewer instead of guessing.
func TestTheRecheckKeepsAPairWithAnArchivedEnd(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	// v7 ids sort by creation, so the first record created is the pair's left end.
	leftGoneA := e.plainContact(ctx, t, "Alex Johnson", "alex.johnson@gone.test")
	leftGone := e.fileOldRulePair(ctx, t, entityContact,
		leftGoneA, e.plainContact(ctx, t, "Alex Meyer", "alex.meyer@gone.test"), 0.88)
	rightGoneA := e.plainContact(ctx, t, "Sam Johnson", "sam.johnson@gone.test")
	rightGoneB := e.plainContact(ctx, t, "Sam Meyer", "sam.meyer@gone.test")
	rightGone := e.fileOldRulePair(ctx, t, entityContact, rightGoneA, rightGoneB, 0.88)
	leadA := e.plainLead(ctx, t, "Alex Johnson", "alex.johnson@gone-leads.test")
	leadPair := e.fileOldRulePair(ctx, t, entityLead,
		leadA, e.plainLead(ctx, t, "Alex Meyer", "alex.meyer@gone-leads.test"), 0.88)
	companyA := e.plainCompany(ctx, t, "Northwind Traders")
	companyPair := e.fileOldRulePair(ctx, t, entityCompany, companyA, e.plainCompany(ctx, t, "Contoso Pharma"), 0.8)

	if _, err := e.store.ArchiveContact(ctx, ids.From[ids.ContactKind](leftGoneA), nil); err != nil {
		t.Fatalf("archive contact: %v", err)
	}
	if _, err := e.store.ArchiveContact(ctx, ids.From[ids.ContactKind](rightGoneB), nil); err != nil {
		t.Fatalf("archive contact: %v", err)
	}
	if _, err := e.store.DisqualifyLead(ctx, ids.From[ids.LeadKind](leadA), DisqualifyLeadInput{}); err != nil {
		t.Fatalf("disqualify lead: %v", err)
	}
	if _, err := e.store.ArchiveCompany(ctx, ids.From[ids.CompanyKind](companyA), nil); err != nil {
		t.Fatalf("archive company: %v", err)
	}

	if got := e.recheck(t); got.Closed != 0 {
		t.Errorf("the pass closed %d pairs whose ends it cannot read, want 0", got.Closed)
	}
	for name, id := range map[string]ids.UUID{
		"contact, left end archived": leftGone, "contact, right end archived": rightGone,
		"lead": leadPair, "company": companyPair,
	} {
		if archived, _ := e.candidateState(ctx, t, id); archived {
			t.Errorf("the %s pair was closed; the pass cannot score it", name)
		}
	}
}

func TestOnlyTheSystemRunsTheRecheck(t *testing.T) {
	e := setupDedupe(t)
	if _, err := e.store.RecheckOpenDedupeCandidates(e.as()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member ran the re-check: err = %v, want ErrPermissionDenied", err)
	}
}
