// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyFactsLeft lists the phone and contact_email values the company still
// states, moved or waiting in the legacy table.
func companyFactsLeft(ctx context.Context, t *testing.T, tx pgx.Tx, company ids.UUID) []string {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT value FROM company_fact
		 WHERE company_id = $1 AND field IN ('phone', 'contact_email') ORDER BY value`, company)
	if err != nil {
		t.Fatal(err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestErasureDeletesTheCompanyFactsThatStateTheSubjectsDetails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	subject, colleague, employer, other, third := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, c := range []ids.UUID{subject, colleague} {
		mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Lan Pham', 'manual', 'test')`, c)
	}
	for _, c := range []ids.UUID{employer, other, third} {
		mustExec(ctx, t, tx, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Voltaq', 'seed', 'system')`, c)
	}
	mustExec(ctx, t, tx, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, 'lan@voltaq.test', 'manual', 'test')`, subject)
	mustExec(ctx, t, tx, `INSERT INTO contact_phone (contact_id, phone, source, captured_by)
		VALUES ($1, '+4930555010', 'manual', 'test')`, subject)
	mustExec(ctx, t, tx, `INSERT INTO contact_phone (contact_id, phone, source, captured_by)
		VALUES ($1, '+4930999888', 'manual', 'test')`, colleague)
	// A mobile number the subject's signature stated, still waiting in the legacy table.
	mustExec(ctx, t, tx, `INSERT INTO contact_profile_field_legacy
		  (contact_id, field, value_key, value, evidence_snippet, source_ref, source, captured_by)
		VALUES ($1, 'phone', '+491701234567', '+49 170 1234567', 'mobile', 'activity:x', 'capture_enrich', 'agent:enrich')`, subject)

	fact := `INSERT INTO field_provenance (object_type, object_id, field_name, value, evidence_snippet, evidence_ref, confidence, source, captured_by)
		VALUES ('company', $1, $2, $3, 'impressum', 'https://voltaq.test/impressum', 0.9, 'site_read', 'agent:deepread')`
	mustExec(ctx, t, tx, fact, employer, "phone", "+49 (30) 555-010")
	mustExec(ctx, t, tx, fact, employer, "contact_email", " lan@voltaq.test")
	mustExec(ctx, t, tx, fact, third, "phone", "+49 30 999 888")
	mustExec(ctx, t, tx, fact, other, "phone", "+49 170 1234567")
	mustExec(ctx, t, tx, `INSERT INTO company_fact_legacy
		  (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'company', 'phone', '0170 1234567', '', 'mobile', 'https://other.test', 0.8, 'site_read', 'agent:deepread'),
		       ($1, 'company', 'contact_email', 'LAN@voltaq.test', '', 'contact', 'https://other.test', 0.8, 'site_read', 'agent:deepread')`, other)
	mustExec(ctx, t, tx, `INSERT INTO company_fact_legacy
		  (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'company', 'contact_email', 'info@other.test', '', 'contact', 'https://other.test', 0.8, 'site_read', 'agent:deepread')`, employer)

	if err := purgeSubjectCompanyEvidence(ctx, tx, subject); err != nil {
		t.Fatalf("purging the subject's company facts: %v", err)
	}

	if left := companyFactsLeft(ctx, t, tx, employer); !slices.Equal(left, []string{"info@other.test"}) {
		t.Errorf("the employer states %v, want only its own address left once the subject's number and address go", left)
	}
	// 0170 1234567 is the subject's mobile in national form, whose digits differ:
	// the match compares digits, so it stays, as a number that may be anyone's.
	if left := companyFactsLeft(ctx, t, tx, other); !slices.Equal(left, []string{"0170 1234567"}) {
		t.Errorf("the other company states %v, want only 0170 1234567 once the subject's mobile and address go", left)
	}
	if left := companyFactsLeft(ctx, t, tx, third); !slices.Equal(left, []string{"+49 30 999 888"}) {
		t.Errorf("the third company states %v, want a colleague's number left alone", left)
	}
}
