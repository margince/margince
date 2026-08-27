// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What an enrichment records, and what that costs the human-edit-precedence
// gate.
//
// compose.HumanOwnedConflicts decides ownership per field by taking the LATEST
// audit row whose after image holds that key, and staging an agent's patch when
// its actor was human. An enrichment that records real column names therefore
// becomes the latest writer of the columns it fills. An image that named only
// its own bookkeeping would hold no column key at all, and could not appear in
// that query however it wrote.
//
// So the protection can no longer rest on the image being uninformative. It
// rests on the writer: applyUnclaimedOrgColumn fills only a column nobody has
// claimed, so an enrichment never holds a key a human typed. That is a property
// of this writer, not of the gate, and it is held here.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// orgAuditImages reads the images of the newest update row for one organization.
func orgAuditImages(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID) (before, after map[string]any) {
	t.Helper()
	var beforeJSON, afterJSON []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(
			ctx,
			`SELECT before, after FROM audit_log
			  WHERE entity_type = 'organization' AND entity_id = $1 AND action = 'update'
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, orgID,
		).Scan(&beforeJSON, &afterJSON)
	}); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if beforeJSON == nil {
		t.Fatal("before is SQL NULL — the row cannot say what the enrichment changed from")
	}
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("before is not an object: %v", err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("after is not an object: %v", err)
	}
	return before, after
}

// A column nobody claimed is the enrichment's to fill, and the row now says what
// it was — which, for a fill, is the absence it replaced.
func TestAnEnrichmentRecordsTheEmptyColumnItFilled(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	if err := e.store.ApplyEnrichment(ctx, orgID, ApplyColdStartProfileInput{
		SourceURL: "https://voltaq.test/about",
		Fields: []ColdStartFieldInput{{
			Field: "industry", Value: "Automotive", Confidence: 0.9,
			EvidenceSnippet: "a supplier to the automotive sector", SourceURL: "https://voltaq.test/about",
		}},
	}); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	before, after := orgAuditImages(ctx, t, e, orgID)
	if value, present := before["industry"]; !present || value != nil {
		t.Errorf("before[industry] = %v (present=%t), want a recorded absence", value, present)
	}
	if after["industry"] != "Automotive" {
		t.Errorf("after[industry] = %v, want the value the fill wrote", after["industry"])
	}
}

// The precedence guarantee itself. A human types the column; the enrichment
// declines to fill it; and because it declined, the column never reaches the
// enrichment's after image — so the human stays its latest writer and an agent's
// patch of it still stages.
//
// The assertion is on the IMAGE rather than on a later agent PATCH because that
// is where the two concerns actually meet: if a machine writer ever did record a
// human-typed column, precedence would flip silently, and no test of the gate
// itself would notice.
func TestAnEnrichmentDoesNotRecordAColumnAHumanTyped(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	_, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	// The human types it first.
	human := "Automotive"
	if _, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{Industry: &human}); err != nil {
		t.Fatalf("human update: %v", err)
	}

	if err := e.store.ApplyEnrichment(ctx, orgID, ApplyColdStartProfileInput{
		SourceURL: "https://voltaq.test/about",
		Fields: []ColdStartFieldInput{{
			Field: "industry", Value: "Aerospace", Confidence: 0.9,
			EvidenceSnippet: "an aerospace parts maker", SourceURL: "https://voltaq.test/about",
		}},
	}); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	before, after := orgAuditImages(ctx, t, e, orgID)
	if _, claimed := after["industry"]; claimed {
		t.Errorf("the enrichment took ownership of a human-typed column: after = %v", after)
	}
	if _, claimed := before["industry"]; claimed {
		t.Errorf("the enrichment recorded a human-typed column it never wrote: before = %v", before)
	}

	var stored string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT industry FROM organization WHERE id = $1`, orgID).Scan(&stored)
	}); err != nil {
		t.Fatalf("reading the column back: %v", err)
	}
	if stored != human {
		t.Errorf("industry = %q, want the human's value untouched", stored)
	}
}
