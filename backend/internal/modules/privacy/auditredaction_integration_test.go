// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPrivacyCertificatesPhysicallyRedactHistoryAndCancelRetainedEvents(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	for _, kind := range []string{"contact", "lead", "activity", "attachment", "deal_room_participant"} {
		t.Run(kind, func(t *testing.T) {
			tx := subjectColumnsTx(ctx, t)
			subject, other := ids.NewV7(), ids.NewV7()
			old := seedPersonalAudit(ctx, t, tx, kind, subject)
			unrelated := seedPersonalAudit(ctx, t, tx, kind, other)
			metadata := auditMetadata(ctx, t, tx, old)
			for _, published := range []bool{false, true} {
				mustExec(ctx, t, tx, `INSERT INTO event_outbox (stream, envelope, published_at)
					VALUES ('gw:events:crm:contact', jsonb_build_object(
					'entity', jsonb_build_object('type', @kind::text, 'id', @id::text),
					'trace', jsonb_build_object('audit_log_id', @audit::text),
					'payload', jsonb_build_object('name', 'Private Contact')),
					CASE WHEN @published THEN now() ELSE NULL END)`,
					pgx.StrictNamedArgs{"kind": kind, "id": subject, "audit": old, "published": published})
			}
			mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
			if err := tombstoneCollateralScrubs(ctx, tx, kind, []ids.UUID{subject}, "test", causeContactErasure); err != nil {
				t.Fatal(err)
			}
			if got := auditMetadata(ctx, t, tx, old); !bytes.Equal(got, metadata) {
				t.Fatalf("audit metadata changed: before=%s after=%s", metadata, got)
			}
			var redacted, untouched bool
			if err := tx.QueryRow(ctx, `SELECT before IS NULL AND after IS NULL
				AND evidence = '{"redacted":true}'::jsonb FROM audit_log WHERE id = $1`, old).Scan(&redacted); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRow(ctx, `SELECT after->>'full_name' = 'Private Contact'
				FROM audit_log WHERE id = $1`, unrelated).Scan(&untouched); err != nil {
				t.Fatal(err)
			}
			var retained int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox
				WHERE envelope->'entity'->>'id' = $1`, subject.String()).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if !redacted || !untouched || retained != 0 {
				t.Fatalf("redacted=%v unrelated untouched=%v retained events=%d", redacted, untouched, retained)
			}
		})
	}
}

func TestAuditPayloadsCannotChangeWithoutACurrentMatchingCertificate(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	for name, statement := range map[string]string{
		"redaction without a certificate": `UPDATE audit_log SET before=NULL, after=NULL, evidence='{"redacted":true}' WHERE id=$1`,
		"replacement payload":             `UPDATE audit_log SET after='{"full_name":"Replacement"}' WHERE id=$1`,
		"metadata":                        `UPDATE audit_log SET action='archive' WHERE id=$1`,
		"deletion":                        `DELETE FROM audit_log WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			tx := subjectColumnsTx(ctx, t)
			old := seedPersonalAudit(ctx, t, tx, "contact", ids.NewV7())
			mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
			if _, err := tx.Exec(ctx, statement, old); err == nil {
				t.Fatal("audit mutation succeeded")
			}
		})
	}
}

func TestErasureProofRedactionKeepsReplayReferencesAndTypedCounts(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, request := ids.NewV7(), ids.NewV7()
	reason := "dsr:" + request.String()
	certificate, err := storekit.AuditWithEvidence(ctx, tx, "erase", "contact", subject, nil, nil, map[string]any{
		"reason": reason, "emails_suppressed": 2,
		"rows": map[string]int64{"contact.updated": 1}, "uncounted": []string{"attachment_object"},
		"decided_by_name": "Private Contact", "future_private_detail": "private@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storekit.RedactAuditHistory(ctx, tx, certificate); err != nil {
		t.Fatal(err)
	}
	var safe bool
	if err := tx.QueryRow(ctx, `SELECT evidence->>'reason'=$2
	 AND evidence->'emails_suppressed'='2'::jsonb AND evidence->'rows'='{"contact.updated":1}'::jsonb
	 AND evidence->'uncounted'='["attachment_object"]'::jsonb
	 AND NOT (evidence ? 'decided_by_name' OR evidence ? 'future_private_detail')
	 FROM audit_log WHERE id=$1`, certificate, reason).Scan(&safe); err != nil {
		t.Fatal(err)
	}
	if !safe {
		t.Fatal("proof lost replay/count metadata or kept personal text")
	}
}

func TestPrivacyReasonProjectionMatchesEvents(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	id := ids.NewV7().String()
	for _, reason := range append(fixedPrivacyReasons(), []string{
		"erase Private Contact at private@example.test", "dsr:" + id, "dsr:invalid", "", "dsr:{" + id + "}",
		"dsr:00000000-0000-0000-0000-000000000000",
	}...) {
		var stored *string
		if err := tx.QueryRow(ctx, `SELECT audit_redacted_evidence(true,
		 jsonb_build_object('reason', $1::text))->>'reason'`, reason).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		event := retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason)
		if !reflect.DeepEqual(stored, event.Reason) {
			t.Errorf("reason %q: certificate=%v event=%v", reason, stored, event.Reason)
		}
	}
}

func seedPersonalAudit(ctx context.Context, t *testing.T, tx pgx.Tx, kind string, subject ids.UUID) ids.UUID {
	t.Helper()
	id, err := storekit.AuditWithEvidence(ctx, tx, "create", kind, subject, nil,
		map[string]string{"full_name": "Private Contact"}, map[string]any{"email": "private@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func auditMetadata(ctx context.Context, t *testing.T, tx pgx.Tx, id ids.UUID) []byte {
	t.Helper()
	var metadata []byte
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(a) - ARRAY['before','after','evidence']
		FROM audit_log a WHERE id = $1`, id).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestFixedPrivacyReasonsMatchTheDatabase(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	var stored []string
	if err := tx.QueryRow(ctx, `SELECT audit_privacy_reason_codes()`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	expected := fixedPrivacyReasons()
	slices.Sort(stored)
	slices.Sort(expected)
	if !slices.Equal(stored, expected) {
		t.Fatalf("database reasons=%q event reasons=%q", stored, expected)
	}
}

func TestPrivacyCertificatesWorkInsideSavepoints(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	original := seedPersonalAudit(ctx, t, tx, "contact", subject)
	mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := storekit.AuditWithEvidence(ctx, nested, "erase", "contact", subject, nil, nil, map[string]any{"reason": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := storekit.RedactAuditHistory(ctx, nested, certificate); err != nil {
		t.Fatal(err)
	}
	if err := nested.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var cleared bool
	if err := tx.QueryRow(ctx, `SELECT before IS NULL AND after IS NULL AND evidence='{"redacted":true}'::jsonb
 FROM audit_log WHERE id=@original`, pgx.StrictNamedArgs{"original": original}).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Fatal("personal audit payload survived a savepoint erasure")
	}
}

func TestErasureKeepsControllerHoldAndReleaseDecisions(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	decisions := make([]ids.UUID, 0, 3)
	for _, action := range []string{"pin", "restrict", "release"} {
		id, err := storekit.AuditWithEvidence(ctx, tx, action, "activity", subject, nil, nil,
			map[string]any{"reason": "keep the signed contract", "decided_by_name": "Controller", "restricted_until": "2030-01-01"})
		if err != nil {
			t.Fatal(err)
		}
		decisions = append(decisions, id)
	}
	certificate, err := storekit.AuditWithEvidence(ctx, tx, "erase", "activity", subject, nil, nil, map[string]any{"reason": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := storekit.RedactAuditHistory(ctx, tx, certificate); err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE id=ANY(@decisions)
 AND evidence->>'reason'='keep the signed contract' AND evidence->>'decided_by_name'='Controller'
 AND evidence->>'restricted_until'='2030-01-01'`, pgx.StrictNamedArgs{"decisions": decisions}).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != len(decisions) {
		t.Fatalf("kept %d controller decisions, want %d", kept, len(decisions))
	}
}

func TestHistoricalAuditCorrectionKeepsHeldActivityAndRequestHistory(t *testing.T) {
	for _, protection := range []string{"legal_hold", "restriction", "floor", "dispute"} {
		t.Run(protection, func(t *testing.T) {
			ctx := principal.SystemActing(context.Background(), "privacy_test")
			tx := subjectColumnsTx(ctx, t)
			subject, kind := ids.NewV7(), "activity"
			if protection == "dispute" {
				kind = "data_subject_request"
				mustExec(ctx, t, tx, `INSERT INTO data_subject_request(id,kind,subject_ref,status,resolution,received_at,due_at,channel,receipt_reference,dispute_open)
     VALUES (@subject,'access','Private Subject','rejected','Private appeal',now(),now()+interval '30 days','phone',@receipt,true)`,
					pgx.StrictNamedArgs{"subject": subject, "receipt": "RCPT-" + subject.String()})
			} else {
				mustExec(ctx, t, tx, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by,restricted_at,archived_at,restricted_reason,restricted_until,retention_class,retention_class_at)
     VALUES (@subject,'email','Private Subject',now(),'manual','system:privacy_test',
      CASE WHEN @restricted THEN now() ELSE NULL END,
 CASE WHEN @restricted THEN now() ELSE NULL END,
 CASE WHEN @restricted THEN 'controller_hold' ELSE NULL END,
 CASE WHEN @restricted THEN now()+interval '100 years' ELSE NULL END,
 CASE WHEN @floor OR @restricted THEN 'commercial_correspondence' ELSE NULL END,
 CASE WHEN @floor OR @restricted THEN now() ELSE NULL END)`,
					pgx.StrictNamedArgs{"subject": subject, "restricted": protection == "restriction", "floor": protection == "floor"})
			}
			if protection == "legal_hold" {
				contact := ids.NewV7()
				mustExec(ctx, t, tx, `INSERT INTO contact(id,full_name,source,captured_by,legal_hold)
     VALUES (@contact,'Held Contact','manual','system:privacy_test',true)`, pgx.StrictNamedArgs{"contact": contact})
				mustExec(ctx, t, tx, `INSERT INTO activity_link(activity_id,entity_type,contact_id) VALUES (@activity,'contact',@contact)`,
					pgx.StrictNamedArgs{"activity": subject, "contact": contact})
			}
			original := seedPersonalAudit(ctx, t, tx, kind, subject)
			if _, err := storekit.AuditWithEvidence(ctx, tx, "erase", kind, subject, nil, nil, map[string]any{"cause": "contact_erasure"}); err != nil {
				t.Fatal(err)
			}
			page, err := RedactHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
			if err != nil {
				t.Fatal(err)
			}
			if page.Changed != 0 {
				t.Fatalf("changed %d held histories", page.Changed)
			}
			var kept bool
			if err := tx.QueryRow(ctx, `SELECT after IS NOT NULL FROM audit_log WHERE id=@original`, pgx.StrictNamedArgs{"original": original}).Scan(&kept); err != nil {
				t.Fatal(err)
			}
			if !kept {
				t.Fatal("historical correction erased held history")
			}
		})
	}
}
