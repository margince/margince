// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDuplicateEvidenceExportsOnlyTheSubjectsSide(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, contactPair, leadPair := seedDedupePairs(ctx, t, tx)
	mustExec(ctx, t, tx, `UPDATE dedupe_candidate SET evidence=jsonb_build_array(jsonb_build_object(
 'field','full_name', 'left_value', CASE WHEN left_contact_id=@contact OR left_lead_id=@lead THEN 'Subject Value' ELSE 'Other Private Value' END,
 'right_value', CASE WHEN right_contact_id=@contact OR right_lead_id=@lead THEN 'Subject Value' ELSE 'Other Private Value' END,
 'future_detail','Other Private Value')) WHERE id=ANY(@pairs)`,
		pgx.StrictNamedArgs{"contact": subject.contact, "lead": subject.lead, "pairs": []ids.UUID{contactPair, leadPair}})
	var pkg SARPackage
	sections, err := sarRetainedCopySections(ctx, &pkg, subject.contact, subject.emails, []ids.UUID{subject.lead})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range sections {
		if section.dest != &pkg.DuplicateEvidence {
			continue
		}
		rows, err := rowMaps(ctx, tx, section.query, section.args...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("exported %d pairs, want both contact and lead pairs", len(rows))
		}
		for _, row := range rows {
			raw, err := json.Marshal(row["evidence"])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "Other Private Value") || !strings.Contains(string(raw), "Subject Value") {
				t.Fatalf("export crossed the subject boundary: %s", raw)
			}
		}
	}
}

func TestLiveErasurePreservesDisputedRightsCases(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	request := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO data_subject_request(id, kind,subject_ref,contact_id,status,resolution,received_at,due_at,channel,receipt_reference,dispute_open)
 VALUES (@request,'access','Private Subject',@subject,'rejected','Private appeal evidence',now(),now()+interval '30 days','phone',@receipt,true)`,
		pgx.StrictNamedArgs{"request": request, "subject": subject.contact, "receipt": "RCPT-" + request.String()})
	original := seedPersonalAudit(ctx, t, tx, "data_subject_request", request)
	if err := retireRightsCases(ctx, tx, subject.contact.UUID, ids.UUID{}); err != nil {
		t.Fatal(err)
	}
	var kept bool
	if err := tx.QueryRow(ctx, `SELECT d.subject_ref='Private Subject' AND d.resolution='Private appeal evidence'
 AND a.after IS NOT NULL FROM data_subject_request d JOIN audit_log a ON a.id=@audit WHERE d.id=@request`,
		pgx.StrictNamedArgs{"request": request, "audit": original}).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if !kept {
		t.Fatal("live erasure changed disputed request data or its history")
	}
}

func TestIntroductionsExportOnlyRequestsAboutTheSubject(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, pair, _ := seedDedupePairs(ctx, t, tx)
	var other, requester ids.UUID
	if err := tx.QueryRow(ctx, `SELECT CASE WHEN left_contact_id=@subject THEN right_contact_id ELSE left_contact_id END,
 split_part(captured_by,':',2)::uuid FROM dedupe_candidate WHERE id=@pair`,
		pgx.StrictNamedArgs{"subject": subject.contact, "pair": pair}).Scan(&other, &requester); err != nil {
		t.Fatal(err)
	}
	introducer := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO app_user(id,email,display_name) VALUES (@user,@email,'Introducer')`,
		pgx.StrictNamedArgs{"user": introducer, "email": introducer.String() + "@sar.test"})
	for _, own := range []bool{true, false} {
		target, through := other, subject.contact.UUID
		note := "Other Private Note"
		if own {
			target, through, note = subject.contact.UUID, other, "Subject Note"
		}
		mustExec(ctx, t, tx, `INSERT INTO intro_request(contact_id,through_contact_id,requester_user_id,introducer_user_id,
   route_type,internal_reason,value_for_target,forwardable_note,due_at,captured_by)
   VALUES (@target,@through,@requester,@introducer,'through_contact',@note,@note,@note,now()+interval '3 days','system:privacy_test')`,
			pgx.StrictNamedArgs{"target": target, "through": through, "requester": requester, "introducer": introducer, "note": note})
	}
	var pkg SARPackage
	sections, err := sarRetainedCopySections(ctx, &pkg, subject.contact, subject.emails, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range sections {
		if section.dest != &pkg.Introductions {
			continue
		}
		rows, err := rowMaps(ctx, tx, section.query, section.args...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["forwardable_note"] != "Subject Note" {
			t.Fatalf("exported someone else's introduction: %+v", rows)
		}
	}
}

func TestDuplicateEvidenceKeepsWorkspaceReadableCustomerIdentity(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	mustExec(ctx, t, tx, `UPDATE contact SET owner_id=split_part(captured_by,':',2)::uuid WHERE id=@contact`, pgx.StrictNamedArgs{"contact": subject.contact})
	mustExec(ctx, t, tx, `UPDATE lead SET owner_id=(SELECT owner_id FROM contact WHERE id=@contact) WHERE id=@lead`, pgx.StrictNamedArgs{"contact": subject.contact, "lead": subject.lead})

	user := ids.NewV7()
	reader := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	})
	var pkg SARPackage
	sections, err := sarRetainedCopySections(reader, &pkg, subject.contact, subject.emails, []ids.UUID{subject.lead})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range sections {
		if section.dest != &pkg.DuplicateEvidence {
			continue
		}
		rows, err := rowMaps(reader, tx, section.query, section.args...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("a workspace reader received %d subject pairs, want two", len(rows))
		}
	}
}
