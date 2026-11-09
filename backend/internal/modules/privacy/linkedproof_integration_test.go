// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHistoricalCorrectionNeedsAValidEarlierErasureCertificate(t *testing.T) {
	for _, mode := range []string{"missing", "malformed", "restriction", "future_entry"} {
		t.Run(mode, func(t *testing.T) {
			ctx := principal.SystemActing(context.Background(), "privacy_test")
			tx := subjectColumnsTx(ctx, t)
			var boundary ids.UUID
			if mode == "future_entry" {
				boundary = legacyContactProof(ctx, t, tx, ids.NewV7())
			}
			original := seedPersonalAudit(ctx, t, tx, "intro_request", ids.NewV7())
			if mode == "restriction" {
				var err error
				boundary, err = storekit.AuditWithEvidence(ctx, tx, "restrict", "contact", ids.NewV7(), nil, nil, map[string]any{"cause": "retention"})
				if err != nil {
					t.Fatal(err)
				}
			}
			evidence := map[string]any{"payload_redaction": true, "legacy_erased_record": true}
			if boundary != (ids.UUID{}) {
				evidence["boundary"] = boundary.String()
			}
			if mode == "malformed" {
				evidence["boundary"] = "not-a-certificate"
			}
			proof, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "audit_log", original, nil, evidence)
			if err != nil {
				t.Fatal(err)
			}
			mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
			if err := storekit.RedactHistoricalAuditEntry(ctx, tx, proof); err == nil {
				t.Fatal("accepted an unproven or later audit entry")
			}
			assertLegacyAuditImage(ctx, t, tx, original, true)
			nested, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = nested.Exec(ctx, `UPDATE audit_log SET before=NULL,after=NULL,evidence='{"redacted":true}' WHERE id=@original`, pgx.StrictNamedArgs{"original": original})
			if err == nil {
				t.Fatal("database accepted a redaction without an earlier erasure certificate")
			}
			if err := nested.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClosingADisputeDoesNotRetireARequestTheErasureKept(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	request := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO data_subject_request(id,kind,subject_ref,contact_id,status,resolution,received_at,due_at,channel,receipt_reference)
 VALUES(@id,'access',@reference,@contact,'fulfilled','Kept during a dispute',now(),now()+interval '30 days','phone',@receipt)`, pgx.StrictNamedArgs{"id": request, "reference": subject.contact.String(), "contact": subject.contact, "receipt": "RCPT-" + request.String()})
	original, err := storekit.AuditWithEvidence(ctx, tx, "create", "data_subject_request", request, nil, map[string]any{"subject_ref": subject.contact.String(), "full_name": "Private Contact", "dispute_open": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	for _, step := range []auditPageForTest{RedactLinkedHistoricalAuditPageTx, RedactLinkedHistoricalIdentitiesPageTx} {
		if _, err := step(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil); err != nil {
			t.Fatal(err)
		}
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

func TestAClosedBlankApprovalNeedsAnIdentifiableErasure(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	approval := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO approval(id,kind,status,proposed_change,diff_hash,expires_at,proposed_by,summary,decided_at,consumed_at,decision_reason)
 VALUES(@id,'contact_update','approved','{}','legacy',timestamptz '2030-01-01','system:legacy','',now(),now(),@reason)`, pgx.StrictNamedArgs{"id": approval, "reason": subjectWithdrawal})
	original := seedPersonalAudit(ctx, t, tx, "approval", approval)
	legacyContactProof(ctx, t, tx, ids.NewV7())
	if _, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil); err != nil {
		t.Fatal(err)
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

func TestHistoricalCorrectionDoesNotRepeatForControllerDecisions(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	for _, action := range []string{"pin", "restrict"} {
		if _, err := storekit.AuditWithEvidence(ctx, tx, action, "contact", subject, nil, nil,
			map[string]any{"reason": "Controller decision", "decided_by_name": "Controller"}); err != nil {
			t.Fatal(err)
		}
	}
	seedPersonalAudit(ctx, t, tx, "contact", subject)
	legacyContactProof(ctx, t, tx, subject)
	until := ids.NewV7()
	for i := range 2 {
		page, err := RedactHistoricalAuditPageTx(ctx, tx, ids.UUID{}, until, 100, nil)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && page.Changed != 1 {
			t.Fatalf("first pass changed %d records, want one", page.Changed)
		}
		if i == 1 && page.Changed != 0 {
			t.Fatalf("replay rewrote %d controller decisions", page.Changed)
		}
	}
}
