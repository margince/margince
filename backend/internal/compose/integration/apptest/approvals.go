// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// DeepReadPayload is what a site read's company proposal carries. It is a real
// one: a decision through the composed application runs the member's accept
// effect, and a placeholder body would report effect_failed.
func DeepReadPayload(companyID, readID string) string {
	return `{"company_id":"` + companyID + `","source_url":"https://acme.example",` +
		`"site_read_id":"` + readID + `","fields":[{"field":"industry","value":"Industrial valves",` +
		`"evidence_snippet":"Acme makes industrial valves.","source_url":"https://acme.example","confidence":0.9}],"facts":[]}`
}

// StageBundleRows puts one act's proposals in the inbox through the owner
// connection. They are the rows a site read leaves behind, so the arrange step
// needs no crawler.
func StageBundleRows(t *testing.T, e *AppEnv, companyID string, members int) ids.UUID {
	t.Helper()
	ctx := context.Background()
	bundle, readID := ids.NewV7(), ids.NewV7()
	// The session's own seat decides these rows, so it is read from the app:
	// any other pick from app_user can stage somebody else's inbox.
	adminID := SessionUserID(t, e)
	for i := range members {
		kind, payload := "deepread", DeepReadPayload(companyID, readID.String())
		if _, err := e.Owner.Exec(ctx, `
			INSERT INTO approval (kind, proposed_by, on_behalf_of, target_entity_type,
			                      target_entity_id, summary, proposed_change, diff_hash, expires_at, bundle_id,
			                      requested_decider_id, requested_decider_source)
			VALUES ($1, 'agent:site-read', $2, 'company', $3, $4, $5::jsonb, $6,
			        now() + interval '1 day', $7, $2, 'on_behalf_of')`,
			kind, adminID, companyID, "Staged by the site read", payload,
			ids.NewV7().String(), bundle); err != nil {
			t.Fatalf("staging member %d (%s): %v", i, kind, err)
		}
	}
	return bundle
}

// SeedExtractionReading writes one finished reading through the production
// store and answers its id, for a suite whose subject is what follows it. Over
// HTTP a reading needs a worker and a model; a hand-written row would prove
// nothing about the writer that fills it.
func SeedExtractionReading(
	e *AppEnv, t *testing.T, attachmentID string, fields []extraction.ExtractedField,
) string {
	t.Helper()
	ctx := e.DealWriterContext(t)
	store := activities.NewStore(e.DB())
	id, err := ids.Parse(attachmentID)
	if err != nil {
		t.Fatalf("parse attachment id %q: %v", attachmentID, err)
	}
	read, _, err := store.StartExtractionReadQueued(ctx, id, "seed", nil)
	if err != nil {
		t.Fatalf("StartExtractionReadQueued: %v", err)
	}
	claim, err := store.BeginExtractionRead(ctx, read.ID, activities.ExtractionReadLease)
	if err != nil {
		t.Fatalf("BeginExtractionRead: %v", err)
	}
	if claim.StartedAt == nil {
		t.Fatal("a claimed reading carries no start time")
	}
	if err := store.FinishExtractionRead(ctx, read.ID, activities.ExtractionReadOutcome{
		Status: activities.ExtractionReadDone, Fields: fields, ClaimedAt: *claim.StartedAt,
	}); err != nil {
		t.Fatalf("FinishExtractionRead: %v", err)
	}
	return read.ID.String()
}
