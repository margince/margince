// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"reflect"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAuditErasureKeepsTypedBusinessHistoryWithoutPersonalValues(t *testing.T) {
	for _, fixture := range []struct {
		kind        string
		image, kept map[string]any
	}{
		{"lead", map[string]any{"status": "promoted", "full_name": "Private Contact", "email": "private@example.test"}, map[string]any{"status": "promoted"}},
		{"data_subject_request", map[string]any{"status": "fulfilled", "resolution": true, "subject_ref": "Private Contact"}, map[string]any{"status": "fulfilled", "resolution": true}},
		{"activity", map[string]any{"occurred_at": "2026-07-15T09:00:00Z", "occurred_precision": "instant", "meeting_status": "canceled", "commitment_admission_redacted": "private@example.test", "body": "Private Contact"}, map[string]any{"occurred_at": "2026-07-15T09:00:00Z", "occurred_precision": "instant", "meeting_status": "canceled", "commitment_admission_redacted": true}},
	} {
		t.Run(fixture.kind, func(t *testing.T) {
			ctx := principal.SystemActing(context.Background(), "privacy_test")
			tx := subjectColumnsTx(ctx, t)
			subject := ids.NewV7()
			original, err := storekit.AuditWithEvidence(ctx, tx, "update", fixture.kind, subject, fixture.image, fixture.image, map[string]any{"email": "private@example.test"})
			if err != nil {
				t.Fatal(err)
			}
			mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
			proof, err := storekit.AuditWithEvidence(ctx, tx, "erase", fixture.kind, subject, nil, nil, map[string]any{"cause": "contact_erasure"})
			if err != nil {
				t.Fatal(err)
			}
			if err := storekit.RedactAuditHistory(ctx, tx, proof); err != nil {
				t.Fatal(err)
			}
			var before, after, evidence map[string]any
			if err := tx.QueryRow(ctx, `SELECT before,after,evidence FROM audit_log WHERE id=@original`, pgx.StrictNamedArgs{"original": original}).Scan(&before, &after, &evidence); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, fixture.kept) || !reflect.DeepEqual(after, fixture.kept) || !reflect.DeepEqual(evidence, map[string]any{"redacted": true}) {
				t.Fatalf("before=%v after=%v evidence=%v, want business history %v", before, after, evidence, fixture.kept)
			}
			page, err := RedactHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
			if err != nil {
				t.Fatal(err)
			}
			if page.Changed != 0 {
				t.Fatalf("replay rewrote %d already redacted histories", page.Changed)
			}
		})
	}
}

func TestRedactedBusinessStatesFollowTheirTableChecks(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	for _, source := range []struct{ kind, field, check string }{
		{"lead", "status", "lead_status_check"},
		{"data_subject_request", "status", "data_subject_request_status_check"},
		{"activity", "meeting_status", "activity_meeting_status_check"},
		{"activity", "occurred_precision", "activity_occurred_precision_check"},
	} {
		var definition string
		if err := tx.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid=to_regclass(@kind) AND conname=@check`, pgx.StrictNamedArgs{"kind": source.kind, "check": source.check}).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		states := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(definition, -1)
		if len(states) < 2 {
			t.Fatalf("short state census: %s", definition)
		}
		for _, state := range states {
			var kept *string
			if err := tx.QueryRow(ctx, `SELECT audit_redacted_image(@kind,jsonb_build_object(@field::text,@value::text))->>@field::text`, pgx.StrictNamedArgs{"kind": source.kind, "field": source.field, "value": state[1]}).Scan(&kept); err != nil {
				t.Fatal(err)
			}
			if kept == nil || *kept != state[1] {
				t.Errorf("lost %s.%s state %q", source.kind, source.field, state[1])
			}
		}
		var rejected bool
		if err := tx.QueryRow(ctx, `SELECT audit_redacted_image(@kind,jsonb_build_object(@field::text,'Private Contact','future_private_detail','private@example.test')) IS NULL`, pgx.StrictNamedArgs{"kind": source.kind, "field": source.field}).Scan(&rejected); err != nil {
			t.Fatal(err)
		}
		if !rejected {
			t.Fatalf("%s.%s kept unknown personal values", source.kind, source.field)
		}
	}
}
