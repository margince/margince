// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

// A site read is the model's text, and nobody can shorten it before it lands.
// So a value and quote past their columns are cut, and the read still applies.
func TestASiteReadValuePastTheLimitLandsCut(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	limit := textbounds.LimitOf("public.company_profile_field.value")
	long := strings.Repeat("ế", limit+10)
	companyID, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{
		SourceURL: "https://longread.example/about",
		Fields: []ColdStartFieldInput{{
			Field: "icp", Value: long, EvidenceSnippet: long,
			SourceURL: "https://longread.example/about", Confidence: 0.9,
		}},
	})
	if err != nil {
		t.Fatalf("a long site read failed the apply: %v", err)
	}
	var value, snippet int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT char_length(value), char_length(evidence_snippet)
			FROM company_profile_field WHERE company_id = $1 AND field = 'icp'`, companyID).Scan(&value, &snippet)
	}); err != nil {
		t.Fatalf("reading the icp row: %v", err)
	}
	if value != limit || snippet != textbounds.LimitOf("public.company_profile_field.evidence_snippet") {
		t.Fatalf("stored %d and %d characters, want both cut to the limit", value, snippet)
	}
}

// A user correcting a field can shorten it, so one past the limit is refused.
func TestAProfileFieldCorrectionPastTheLimitIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := evidenceCompany(ctx, t, e)
	limit := textbounds.LimitOf("public.company_profile_field.value")
	corrected := strings.Repeat("a", limit+1)

	_, err := e.store.UpdateCompanyProfileField(ctx, companyID, "icp", ProfileFieldWriteInput{Value: &corrected})
	column, max, ok := storekit.TextBoundViolation(err)
	if !ok || column != "value" || max != limit {
		t.Fatalf("the correction answered %v, want the value column refused at %d", err, limit)
	}
}
