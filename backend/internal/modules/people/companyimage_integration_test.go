// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What a company save records, and where the form's bookkeeping goes.
//
// The company form re-sends every field on every save, so the write statements
// are all guarded by IS DISTINCT FROM and report only whether they moved
// something. The images are read either side of them, which is what makes an
// undo of a mistyped save possible at all — and it is a human typing, so these
// are the rows that make the human the latest writer of the fields they typed.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// anchorAuditEvidence reads the evidence of the newest company update row —
// the half of the write that carries context ABOUT the save.
func anchorAuditEvidence(ctx context.Context, t *testing.T, s *Store, orgID ids.OrganizationID) map[string]any {
	t.Helper()
	var evidence map[string]any
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx,
			`SELECT evidence FROM audit_log
			  WHERE entity_type = 'organization' AND entity_id = $1 AND action = 'update'
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, orgID).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &evidence)
	}); err != nil {
		t.Fatalf("reading the company save's evidence: %v", err)
	}
	return evidence
}

// A save that renames the company and re-answers its industry says what both
// held, and keeps the form's own bookkeeping out of the images: `anchor` and
// `source` are not fields of a company, and field history would project them
// as though they were.
func TestACompanySaveRecordsWhatTheFormReplaced(t *testing.T) {
	env := newAnchorEnv(t)
	automotive := "Automotive"
	if _, err := env.store.SaveCompany(env.ctx, SaveCompanyInput{
		DisplayName: "Our Company",
		Fields:      map[string]*string{fieldIndustry: &automotive},
	}); err != nil {
		t.Fatalf("the save that types the first answers: %v", err)
	}

	renewables := "Renewables"
	if _, err := env.store.SaveCompany(env.ctx, SaveCompanyInput{
		DisplayName: "Nordwind Systeme GmbH",
		Fields:      map[string]*string{fieldIndustry: &renewables},
	}); err != nil {
		t.Fatalf("the save under test: %v", err)
	}

	before, after := auditImagesHolding(env.ctx, t, env.store, "organization", env.anchorID.UUID, fieldDisplayName)
	wantImage(t, before, "before", fieldDisplayName, "Our Company")
	wantImage(t, after, "after", fieldDisplayName, "Nordwind Systeme GmbH")
	wantImage(t, before, "before", fieldIndustry, automotive)
	wantImage(t, after, "after", fieldIndustry, renewables)
	for _, key := range []string{auditKeySource, "anchor", auditKeyFields} {
		if _, folded := after[key]; folded {
			t.Errorf("the after image carries %q, which is not a field of the company: %v", key, after)
		}
	}

	evidence := anchorAuditEvidence(env.ctx, t, env.store, env.anchorID)
	if evidence["anchor"] != true {
		t.Errorf("evidence = %v, want it to record that this is the installation's own company", evidence)
	}
	if evidence[auditKeySource] != companySourceHuman {
		t.Errorf("evidence[%s] = %v, want %q", auditKeySource, evidence[auditKeySource], companySourceHuman)
	}
	if evidence[auditKeyFields] == nil {
		t.Errorf("evidence = %v, want the fields the submission touched", evidence)
	}
}

// The form re-sends every field it was shown, so a save that changes one thing
// must not present the rest as edits: an image narrowed to what moved is the
// difference between a history a person can read and one page of noise per save.
func TestACompanySaveRecordsOnlyTheFieldsThatMoved(t *testing.T) {
	env := newAnchorEnv(t)
	automotive := "Automotive"
	summary := "We keep fleets on the road."
	if _, err := env.store.SaveCompany(env.ctx, SaveCompanyInput{
		DisplayName: "Our Company",
		Fields:      map[string]*string{fieldIndustry: &automotive, fieldOfferSummary: &summary},
	}); err != nil {
		t.Fatalf("the save that types the first answers: %v", err)
	}

	// The same submission again, with one word changed.
	renewables := "Renewables"
	if _, err := env.store.SaveCompany(env.ctx, SaveCompanyInput{
		DisplayName: "Our Company",
		Fields:      map[string]*string{fieldIndustry: &renewables, fieldOfferSummary: &summary},
	}); err != nil {
		t.Fatalf("the save under test: %v", err)
	}

	before, after := auditImagesHolding(env.ctx, t, env.store, "organization", env.anchorID.UUID, fieldIndustry)
	wantImage(t, before, "before", fieldIndustry, automotive)
	wantImage(t, after, "after", fieldIndustry, renewables)
	for _, key := range []string{fieldDisplayName, fieldOfferSummary} {
		if _, moved := after[key]; moved {
			t.Errorf("the after image presents %q as a change, and the submission re-sent it unchanged: %v", key, after)
		}
	}
}
