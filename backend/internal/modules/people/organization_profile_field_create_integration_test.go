// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Stating a profile field the machine never proposed.
//
// Most of this vocabulary is only ever filled by a crawl that found a legal
// notice. A company whose site prints no imprint has no row for register_vat,
// and the correction path — which locates a row and patches it — answered 404.
// A rep who knew the number could not record it, and the VAT consultation that
// a written number is supposed to queue was unreachable for exactly the
// companies a person would want to check.
//
// What must NOT follow from that is a confirm verb that invents rows: agreeing
// with a claim nobody made is not an act the product has.

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bareOrg seeds a company with NO sidecar rows at all — the state a crawl that
// found no imprint leaves, and the one evidenceOrg deliberately does not model.
func bareOrg(ctx context.Context, t *testing.T, e *dedupeEnv) ids.OrganizationID {
	t.Helper()
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Halden Kraft GmbH", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "halden-kraft.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	return ids.From[ids.OrganizationKind](ids.UUID(org.Id))
}

func profileFieldSource(
	ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID, field string,
) string {
	t.Helper()
	var source string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT source FROM organization_profile_field
			 WHERE organization_id = $1 AND field = $2`, orgID, field).Scan(&source)
	}); err != nil {
		t.Fatalf("read profile field %s source: %v", field, err)
	}
	return source
}

// The case the product could not do: a person states a fact about a company
// whose site never published it.
func TestARepCanStateAProfileFieldTheMachineNeverProposed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	stated := "DE811907980"
	out, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "register_vat",
		ProfileFieldWriteInput{Value: &stated})
	if err != nil {
		t.Fatalf("state a VAT number on a company with no sidecar row: %v", err)
	}

	if out.Value != stated {
		t.Errorf("value = %q, want %q", out.Value, stated)
	}
	// A created row belongs to the person who created it, on both columns the
	// enrichment upserts read: source decides what the reader is told, and
	// captured_by is what stops the next crawl reclaiming the row.
	if string(out.Source) != "human" {
		t.Errorf("source = %q, want human — the person IS the evidence on this path", out.Source)
	}
	if got := profileFieldSource(ctx, t, e, orgID, "register_vat"); got != "human" {
		t.Errorf("stored source = %q, want human", got)
	}
	if out.VerifiedAt == nil || out.VerifiedBy == nil {
		t.Fatal("a stated value records who stated it")
	}
	if *out.VerifiedBy != e.rep.String() {
		t.Errorf("verified_by = %v, want the calling rep %v", *out.VerifiedBy, e.rep)
	}
}

// The boundary the create must not cross. A confirmation says "I read this and
// it is right"; there is nothing to read, and a row invented to be agreed with
// would record a human verdict on a value nobody ever proposed.
func TestConfirmingAProfileFieldNobodyProposedIsStillNotFound(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	_, err := e.store.ConfirmOrganizationProfileField(ctx, orgID, "register_vat",
		ProfileFieldWriteInput{})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want not-found — a confirmation cannot create the claim it agrees with", err)
	}

	var rows int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM organization_profile_field
			 WHERE organization_id = $1 AND field = 'register_vat'`, orgID).Scan(&rows)
	}); err != nil {
		t.Fatalf("count profile field rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("the refused confirmation left %d row(s) behind", rows)
	}
}

// The reason the create matters: a stated number is an unchecked number, and
// the consultation has to be queued by the write that stated it. Queued in the
// SAME transaction, so a correction that rolls back leaves no job asking about
// a number the record does not hold.
func TestStatingAVatNumberQueuesItsConsultation(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	var queued []ids.OrganizationID
	e.store.WithVatCheckEnqueue(func(_ context.Context, _ pgx.Tx, id ids.OrganizationID) error {
		queued = append(queued, id)
		return nil
	})

	stated := "DE811907980"
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "register_vat",
		ProfileFieldWriteInput{Value: &stated}); err != nil {
		t.Fatalf("state a VAT number: %v", err)
	}

	if len(queued) != 1 {
		t.Fatalf("queued %d consultations, want exactly 1 — a stated number nobody checks is worse than none", len(queued))
	}
	if queued[0] != orgID {
		t.Errorf("queued the consultation for %v, want %v", queued[0], orgID)
	}
}

// The agent principal is dedupequeue's own `asAgent` — it already carries the
// organization update grant, so the only thing separating it from `as()` here
// is who is acting, which is what these two cases turn on.
//
// Originating a claim is a human act. This endpoint runs at auto_execute, so
// without the guard an agent could mint a legal-identity claim on a company
// nothing was ever read about — and writeEvidence stamps verified_by with the
// user who granted the agent, recording a human verification of a value that
// human never saw. Correcting an EXISTING claim stays open to an agent.
func TestAnAgentCannotOriginateAClaimNobodyProposed(t *testing.T) {
	e := setupDedupe(t)
	orgID := bareOrg(e.as(), t, e)

	stated := "DE811907980"
	_, err := e.store.UpdateOrganizationProfileField(e.asAgent(), orgID, "register_vat",
		ProfileFieldWriteInput{Value: &stated})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want not-found — the refusal an agent saw before the create arm existed", err)
	}

	var rows int
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(), `
			SELECT count(*) FROM organization_profile_field
			 WHERE organization_id = $1 AND field = 'register_vat'`, orgID).Scan(&rows)
	}); err != nil {
		t.Fatalf("count profile field rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("the refused agent write minted %d row(s)", rows)
	}
}

// The other half, and the one that stops the guard being a blanket refusal: an
// agent correcting a claim a crawl already made is the ordinary case this
// endpoint exists for, and it must keep working.
func TestAnAgentStillCorrectsAClaimThatAlreadyExists(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := evidenceOrg(ctx, t, e)

	corrected := "Mid-market industrial manufacturers"
	if _, err := e.store.UpdateOrganizationProfileField(e.asAgent(), orgID, "icp",
		ProfileFieldWriteInput{Value: &corrected}); err != nil {
		t.Fatalf("an agent correcting an existing claim: %v", err)
	}
	if got := readProfileFieldValue(ctx, t, e, orgID, "icp"); got != corrected {
		t.Errorf("value = %q, want %q", got, corrected)
	}
}

// The audit trail has to say what happened. A creation recorded as an update
// carries a before-image of the empty row the write itself just inserted — a
// state nobody ever wrote — and "what did it say before I changed it" then
// answers with something the system invented.
func TestAStatedClaimIsAuditedAsACreationWithNoBeforeImage(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	stated := "DE811907980"
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "register_vat",
		ProfileFieldWriteInput{Value: &stated}); err != nil {
		t.Fatalf("state a VAT number: %v", err)
	}

	var (
		action string
		before *string
		after  map[string]any
	)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT action, before::text, after
			  FROM audit_log
			 WHERE entity_type = 'organization_profile_field'
			 ORDER BY occurred_at DESC LIMIT 1`).Scan(&action, &before, &after)
	}); err != nil {
		t.Fatalf("read the audit row: %v", err)
	}

	if action != "create" {
		t.Errorf("action = %q, want create — the row did not exist before this write", action)
	}
	if before != nil {
		t.Errorf("before = %q, want no image at all — there was no prior state to record", *before)
	}
	// The after image still has to carry what was stated, or the audit records
	// a creation of nothing.
	if after[auditKeyValue] != stated {
		t.Errorf("after[value] = %v, want %q", after[auditKeyValue], stated)
	}
}

// The correction of an existing claim keeps its before-image: that is the half
// the audit trail already had, and a fix to the create path must not cost it.
func TestCorrectingAnExistingClaimStillRecordsWhatItSaidBefore(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := evidenceOrg(ctx, t, e)

	corrected := "Mid-market industrial manufacturers"
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "icp",
		ProfileFieldWriteInput{Value: &corrected}); err != nil {
		t.Fatalf("correct icp: %v", err)
	}

	var (
		action string
		before map[string]any
	)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT action, before
			  FROM audit_log
			 WHERE entity_type = 'organization_profile_field'
			 ORDER BY occurred_at DESC LIMIT 1`).Scan(&action, &before)
	}); err != nil {
		t.Fatalf("read the audit row: %v", err)
	}

	if action != "update" {
		t.Errorf("action = %q, want update", action)
	}
	if before[auditKeyValue] != "Energy-intensive manufacturers" {
		t.Errorf("before[value] = %v, want the machine's original claim", before[auditKeyValue])
	}
}

// Several reps stating the same field at once. What this holds is the OUTCOME:
// every writer succeeds and the field ends up holding one of the stated values
// rather than the empty string the insert seeds — so a first statement never
// leaves a blank claim behind, however the writers interleave.
//
// What it does NOT hold is insertHumanProfileField's ON CONFLICT DO NOTHING:
// this passes with that clause removed, so the collision it guards against is
// not reached here. The clause says why it stays; this says why the test does
// not prove it, rather than reading like it does.
func TestRepsStatingTheSameFieldAtOnceAllSucceed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	stated := []string{
		"DE811907980", "DE136695976", "DE129273398", "DE811128135",
		"DE114110165", "DE811115368", "DE813501887", "DE167015661",
	}
	errs := make([]error, len(stated))
	// Released together, so every writer reaches the insert in the same instant
	// rather than trickling in behind whoever the scheduler started first.
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, value := range stated {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = e.store.UpdateOrganizationProfileField(e.as(), orgID,
				"register_vat", ProfileFieldWriteInput{Value: &value})
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("rep %d stating %q: %v", i, stated[i], err)
		}
	}
	got := readProfileFieldValue(ctx, t, e, orgID, "register_vat")
	if !slices.Contains(stated, got) {
		t.Errorf("value = %q, want one of the stated numbers — the seeded empty row survived", got)
	}
}

// The registry address is the other field a person can now state, and it must
// NOT reach the six address columns: those describe where the company operates.
func TestStatingTheRegisteredAddressLeavesTheOperatingAddressAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := bareOrg(ctx, t, e)

	stated := "Amtsgericht Charlottenburg, Kaiserdamm 1, 14057 Berlin"
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, "registered_address",
		ProfileFieldWriteInput{Value: &stated}); err != nil {
		t.Fatalf("state the registered address: %v", err)
	}

	for _, column := range []string{"address_line1", "address_city", "address_postal_code"} {
		if got := orgColumn(ctx, t, e, orgID, column); got != "" {
			t.Errorf("%s = %q, want empty — the registry address is not the operating address", column, got)
		}
	}
	if got := readProfileFieldValue(ctx, t, e, orgID, "registered_address"); got != stated {
		t.Errorf("registered_address = %q, want %q", got, stated)
	}
}
