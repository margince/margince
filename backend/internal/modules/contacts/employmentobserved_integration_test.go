// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Capture dates the employment edges it plants with the earliest mail that
// placed the contact at the employer's domain, in first_observed_at. The
// account reach walk bounds the employment arm on it, so a date that is too
// late hides real correspondence from the account.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// firstObservedOf reads the first observation on a contact's employment edge
// to a company, failing when there is no such edge.
func (e *dedupeEnv) firstObservedOf(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT first_observed_at FROM relationship
			 WHERE kind = 'employment' AND contact_id = $1 AND company_id = $2 AND archived_at IS NULL`,
			contact, company).Scan(&at)
	}); err != nil {
		t.Fatalf("reading the employment edge's first observation: %v", err)
	}
	return at
}

// observationAudits counts the audit rows that moved an edge's first
// observation.
func (e *dedupeEnv) observationAudits(ctx context.Context, t *testing.T) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'relationship' AND action = 'update'
			   AND after ? 'first_observed_at'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the first-observation audit rows: %v", err)
	}
	return n
}

func mailDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 9, 0, 0, 0, time.UTC)
}

func requireObservedAt(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("the employment edge has no first observation, want %v", want)
	}
	if !got.Equal(want) {
		t.Fatalf("the employment edge was first observed %v, want %v", *got, want)
	}
}

// The message that happens to plant the edge is arbitrary: a backfill hands
// back recent mail first. Older mail from the same address already in the
// workspace is earlier evidence, and mail from another domain is none.
func TestAPlantedEdgeIsDatedFromTheEarliestMailOnItsDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	e.datedEnsureInput(ctx, t, "ann@oldco.test", "oldco.test", mailDay(2023, time.May, 1))
	e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2025, time.January, 10))
	trigger := e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2026, time.March, 2))

	res, err := e.store.EnsureCounterparty(ctx, trigger)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, company), mailDay(2025, time.January, 10))
}

// An edge already in place is dated earlier by an older message captured
// after it, and a newer one leaves it alone. Only the move is audited.
func TestAnOlderMessageMovesTheFirstObservationEarlier(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2026, time.March, 2)))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, company), mailDay(2026, time.March, 2))

	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2025, time.June, 3))); err != nil {
		t.Fatalf("ensure the older message: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, company), mailDay(2025, time.June, 3))
	if n := e.observationAudits(ctx, t); n != 1 {
		t.Fatalf("%d audit rows for the first observation, want the one move", n)
	}
	e.requireObservationAuditShape(ctx, t)

	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2026, time.May, 4))); err != nil {
		t.Fatalf("ensure the newer message: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, company), mailDay(2025, time.June, 3))
	if n := e.observationAudits(ctx, t); n != 1 {
		t.Fatalf("%d audit rows for the first observation, want only the one move", n)
	}
}

// The domain verdict plants the edges for everybody who wrote while the
// question was open, and dates each from that person's own mail.
func TestTheDomainVerdictDatesTheEdgesItPlants(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ben@triaged.test", "triaged.test", mailDay(2025, time.February, 5)))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	readID := e.startTriageRead(ctx, t, "triaged.test")
	verdict, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "triaged.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Triaged GmbH", SeedURL: "https://triaged.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if verdict.CompanyID == nil {
		t.Fatalf("resolve = %+v, want a company", verdict)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, *verdict.CompanyID), mailDay(2025, time.February, 5))
}

// requireObservationAuditShape holds the audit row to the one column that
// moved. What moved it is evidence, not a field.
func (e *dedupeEnv) requireObservationAuditShape(ctx context.Context, t *testing.T) {
	t.Helper()
	var afterKeys, origin string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT (SELECT string_agg(k, ',' ORDER BY k) FROM jsonb_object_keys(after) k),
			       coalesce(evidence->>'origin', '')
			  FROM audit_log
			 WHERE entity_type = 'relationship' AND action = 'update'
			   AND after ? 'first_observed_at'
			 ORDER BY occurred_at DESC LIMIT 1`).Scan(&afterKeys, &origin)
	}); err != nil {
		t.Fatalf("reading the first-observation audit row: %v", err)
	}
	if afterKeys != "first_observed_at" || origin != relationshipOriginCapture {
		t.Fatalf("after-image keys %q with evidence origin %q, want only first_observed_at and origin %q",
			afterKeys, origin, relationshipOriginCapture)
	}
}

// editorCtx is the seed principal with the relationship and merge grants the
// cases below write through.
func (e *dedupeEnv) editorCtx() context.Context {
	ctx := e.as()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return principal.WithActor(ctx, actor)
}

// An employment serving its notice is still current, so a message older than
// its first observation still dates it.
func TestAnEmploymentServingNoticeIsStillDatedByOlderMail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	company := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2026, time.March, 2)))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	leaving := time.Now().AddDate(0, 1, 0).UTC().Truncate(24 * time.Hour)
	if _, err := e.store.UpdateRelationship(ctx, e.employmentEdgeOf(ctx, t, res.ContactID, company),
		UpdateRelationshipInput{EndedAt: &leaving}); err != nil {
		t.Fatalf("recording the notice period: %v", err)
	}
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2025, time.June, 3))); err != nil {
		t.Fatalf("ensure the older message: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, company), mailDay(2025, time.June, 3))
}

// A contact merge archives the duplicate employment and keeps the earlier of
// the two first observations on the edge that survives.
func TestAContactMergeKeepsTheEarliestFirstObservation(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	company := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	kept, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2026, time.March, 2)))
	if err != nil {
		t.Fatalf("ensure the surviving contact: %v", err)
	}
	merged, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "a.smith@newco.test", "newco.test", mailDay(2025, time.January, 10)))
	if err != nil {
		t.Fatalf("ensure the merged contact: %v", err)
	}
	if _, err := e.store.MergeContact(ctx, merged.ContactID, kept.ContactID, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, kept.ContactID, company), mailDay(2025, time.January, 10))
}

// A company merge does the same for a contact employed at both companies.
func TestACompanyMergeKeepsTheEarliestFirstObservation(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	source := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	target := e.seedCompanyOnDomain(ctx, t, "Newco Ltd", "")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", mailDay(2025, time.January, 10)))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	notPrimary := false
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &res.ContactID, CompanyID: &target,
		IsCurrentPrimary: &notPrimary, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the second employment: %v", err)
	}
	if _, err := e.store.MergeCompany(ctx, source, target, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	requireObservedAt(t, e.firstObservedOf(ctx, t, res.ContactID, target), mailDay(2025, time.January, 10))
}

// employmentEdgeOf names the live employment edge between a contact and a
// company.
func (e *dedupeEnv) employmentEdgeOf(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id FROM relationship
			 WHERE kind = 'employment' AND contact_id = $1 AND company_id = $2 AND archived_at IS NULL`,
			contact, company).Scan(&id)
	}); err != nil {
		t.Fatalf("finding the employment edge: %v", err)
	}
	return id
}
