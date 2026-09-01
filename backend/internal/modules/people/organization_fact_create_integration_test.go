// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Stating a fact by hand, and removing one. Both verbs run against real
// Postgres because what has to hold is invisible to the unit gate: the
// vocabulary CHECK, the value_key cardinality CHECK, the audit row's action
// and before-image, and the version precondition the removal shares with a
// correction.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// factRow reads back what the store actually wrote, which is the only account
// of the row a test should trust: asserting on the value the writer returned
// would pass even if nothing reached the table.
func factRow(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID, field, valueKey string) (value, source, capturedBy string, found bool) {
	t.Helper()
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT value, source, captured_by FROM organization_fact
			 WHERE organization_id = $1 AND field = $2 AND value_key = $3`,
			orgID, field, valueKey).Scan(&value, &source, &capturedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil {
		t.Fatalf("read back fact %s:%s: %v", field, valueKey, err)
	}
	return value, source, capturedBy, found
}

func TestAStatedFactIsHumanOwnedFromBirth(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	out, err := e.store.CreateOrganizationFact(ctx, orgID, FactCreateInput{
		Category: "signal", Field: "technology", Value: "Kubernetes",
	})
	if err != nil {
		t.Fatalf("state fact: %v", err)
	}
	if out.Value != "Kubernetes" {
		t.Fatalf("returned value = %q, want Kubernetes", out.Value)
	}

	// captured_by is what BOTH enrichment upserts test before overwriting a
	// row, so a stated fact that did not land human-owned would be silently
	// reclaimed by the next ordinary site read — the defect this assertion
	// exists for, and one the returned struct cannot show.
	value, source, capturedBy, found := factRow(ctx, t, e, orgID, "technology", "kubernetes")
	if !found {
		t.Fatal("stated fact did not reach the table")
	}
	if value != "Kubernetes" || source != "human" {
		t.Fatalf("stored value/source = %q/%q, want Kubernetes/human", value, source)
	}
	if capturedBy != "human:"+e.rep.String() {
		t.Fatalf("captured_by = %q, want the authenticated principal", capturedBy)
	}
}

func TestAStatedFactDerivesItsOwnDedupeKey(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	// A multi-value field keys on the normalized name before the separator;
	// a single-value one carries the empty key its cardinality CHECK demands.
	// Both are derived here rather than supplied, so neither can name a row
	// the value does not belong to.
	if _, err := e.store.CreateOrganizationFact(ctx, orgID, FactCreateInput{
		Category: "offering", Field: "product", Value: "Voltaq Grid — load balancing",
	}); err != nil {
		t.Fatalf("state multi-value fact: %v", err)
	}
	if _, _, _, found := factRow(ctx, t, e, orgID, "product", "voltaq grid"); !found {
		t.Fatal("multi-value fact did not key on its normalized name")
	}

	if _, err := e.store.CreateOrganizationFact(ctx, orgID, FactCreateInput{
		Category: "company", Field: "founded_year", Value: "2011",
	}); err != nil {
		t.Fatalf("state single-value fact: %v", err)
	}
	if _, _, _, found := factRow(ctx, t, e, orgID, "founded_year", ""); !found {
		t.Fatal("single-value fact did not carry the empty value_key its CHECK requires")
	}
}

func TestStatingAFactTheCompanyAlreadyStatesIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	// The seed already carries signal/certification "ISO 9001". Upserting here
	// would let a hand write overwrite a machine claim with none of the
	// correction path's before-image, so the honest verbs are named instead.
	_, err := e.store.CreateOrganizationFact(ctx, orgID, FactCreateInput{
		Category: "signal", Field: "certification", Value: "ISO 9001",
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("restating an existing fact: %v, want a conflict", err)
	}
	// And the machine's claim is untouched: a refused write that had already
	// changed the row would be worse than one that succeeded.
	_, source, _, _ := factRow(ctx, t, e, orgID, "certification", "iso 9001")
	if source != "site_read" {
		t.Fatalf("source after a refused restatement = %q, want site_read", source)
	}
}

func TestAFactOutsideTheVocabularyIsRefusedBeforeTheCheckConstraint(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	// A field that belongs to another category. The row CHECK would refuse it
	// too, but as a 500 naming a constraint; this is the reader's answer.
	_, err := e.store.CreateOrganizationFact(ctx, orgID, FactCreateInput{
		Category: "company", Field: "certification", Value: "ISO 9001",
	})
	if err == nil {
		t.Fatal("a market field under company was accepted")
	}
	// The point is WHICH refusal. The row CHECK would also reject this, as a
	// 500 naming a constraint, and a test accepting any error would pass on
	// exactly the outcome this validation exists to prevent.
	var parse *values.ParseError
	if !errors.As(err, &parse) {
		t.Fatalf("vocabulary refusal = %v, want a ParseError the reader can act on", err)
	}
	if parse.Field != evidenceFieldKey {
		t.Fatalf("refusal names %q, want the field that was wrong", parse.Field)
	}
}

func TestRemovingAFactLeavesTheRestOfTheEvidenceStanding(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	if err := e.store.DeleteOrganizationFact(ctx, orgID,
		"certification:iso 9001", FactWriteInput{}); err != nil {
		t.Fatalf("remove fact: %v", err)
	}
	if _, _, _, found := factRow(ctx, t, e, orgID, "certification", "iso 9001"); found {
		t.Fatal("the removed fact is still in the table")
	}
	// The sibling fact is untouched: a removal that took the whole category
	// with it would pass a test asserting only the target's absence.
	if _, _, _, found := factRow(ctx, t, e, orgID, "phone", ""); !found {
		t.Fatal("removing one fact took another with it")
	}

	facts, err := e.store.ListOrganizationFacts(ctx, orgID)
	if err != nil {
		t.Fatalf("list after removal: %v", err)
	}
	for _, fact := range facts {
		if fact.Field == "certification" {
			t.Fatal("the removed fact is still listed")
		}
	}
}

func TestRemovingAFactRecordsWhatWasRemoved(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	if err := e.store.DeleteOrganizationFact(ctx, orgID,
		"certification:iso 9001", FactWriteInput{}); err != nil {
		t.Fatalf("remove fact: %v", err)
	}

	// The row is gone, so the audit before-image is the only account left of
	// what the company was once said to hold. A deletion nobody can read back
	// is the one this must not become.
	var action string
	var before []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT action, before FROM audit_log
			 WHERE entity_type = 'organization_fact'
			 ORDER BY occurred_at DESC LIMIT 1`).Scan(&action, &before)
	}); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if action != "delete" {
		t.Fatalf("audit action = %q, want delete", action)
	}
	// The value alone is not enough to answer "what did this company lose":
	// the row is gone, so entity_id points at nothing, and the image has to
	// name the organization and the claim as well.
	var image map[string]any
	if err := json.Unmarshal(before, &image); err != nil {
		t.Fatalf("audit before-image is not readable: %v", err)
	}
	for _, key := range []string{"value", "organization_id", "category", "field", "value_key"} {
		if _, ok := image[key]; !ok {
			t.Fatalf("audit before-image %s omits %q, so the removal cannot be read back", before, key)
		}
	}
	if image["value"] != "ISO 9001" {
		t.Fatalf("audit before-image value = %v, want the removed claim", image["value"])
	}
}

func TestRemovingAFactHonoursTheVersionTheCallerSaw(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	// A version nobody ever wrote: the caller is acting on a row that has
	// moved under them, and the removal must refuse rather than delete what
	// they never saw.
	stale := int64(99)
	err := e.store.DeleteOrganizationFact(ctx, orgID,
		"certification:iso 9001", FactWriteInput{IfVersion: &stale})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale precondition: %v, want a version conflict the caller can retry", err)
	}
	if _, _, _, found := factRow(ctx, t, e, orgID, "certification", "iso 9001"); !found {
		t.Fatal("a refused removal deleted the row anyway")
	}
}

func TestRemovingAFactThatIsNotThereIsNotFound(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedOrgWithEvidence(ctx, t, e)

	err := e.store.DeleteOrganizationFact(ctx, orgID,
		"technology:nothing here", FactWriteInput{})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("removing an absent fact: %v, want not-found", err)
	}
}
