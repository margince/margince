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
	sections, err := sarRetainedCopySections(ctx, &pkg, []ids.UUID{subject.contact.UUID}, subject.emails, []ids.UUID{subject.lead})
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

func TestIntroductionsExportTheSubjectsOwnRoleAndWithholdOtherTargetsNotes(t *testing.T) {
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
	sections, err := sarRetainedCopySections(ctx, &pkg, []ids.UUID{subject.contact.UUID}, subject.emails, nil)
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
		if len(rows) != 2 {
			t.Fatalf("subject introduction rows=%+v, want target and intermediary", rows)
		}
		for _, row := range rows {
			if row["subject_role"] == "target" {
				if row["forwardable_note"] != "Subject Note" {
					t.Fatalf("subject's own note is missing: %+v", row)
				}
				continue
			}
			if row["subject_role"] != "intermediary" {
				t.Fatalf("unknown subject role: %+v", row)
			}
			for _, field := range []string{"contact_id", "forwardable_note", "value_for_target", "internal_reason", "decision_reason"} {
				if row[field] != nil {
					t.Fatalf("intermediary export disclosed target field %s: %+v", field, row)
				}
			}
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
	sections, err := sarRetainedCopySections(reader, &pkg, []ids.UUID{subject.contact.UUID}, subject.emails, []ids.UUID{subject.lead})
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

// The export reaches the retained history of every record it calls the
// subject's. A contact merged into them keeps its own id. A lead holding their
// address was never promoted. Mutation: match only the survivor's id, or reach only
// promoted leads, and that history is left out.
func TestRetainedHistoryReachesMergedRecordsAndAddressMatchedLeads(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	predecessor, addressLead := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by, merged_into_id)
 VALUES (@id, 'Hedda Before', 'manual', 'system:privacy_test', @subject)`,
		pgx.StrictNamedArgs{"id": predecessor, "subject": subject.contact})
	mustExec(ctx, t, tx, `INSERT INTO lead (id, full_name, email, source, captured_by, archived_at)
 VALUES (@id, 'Hedda Lead', @email, 'manual', 'system:privacy_test', now())`,
		pgx.StrictNamedArgs{"id": addressLead, "email": subject.emails[0]})
	want := map[string]bool{
		seedPersonalAudit(ctx, t, tx, "contact", predecessor).String(): false,
		seedPersonalAudit(ctx, t, tx, "lead", addressLead).String():    false,
	}
	emails, leads, identities, err := subjectReach(ctx, tx, subject.contact)
	if err != nil {
		t.Fatal(err)
	}
	var pkg SARPackage
	sections, err := sarRetainedCopySections(ctx, &pkg, identities, emails, leads)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range sections {
		if section.dest != &pkg.AuditHistory {
			continue
		}
		rows, err := rowMaps(ctx, tx, `SELECT a.id FROM audit_log a
 WHERE (a.entity_type, a.entity_id, a.occurred_at) IN (SELECT entity_type, entity_id, occurred_at FROM (`+section.query+`) h)`,
			section.args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if id, ok := row["id"].(string); ok {
				if _, seeded := want[id]; seeded {
					want[id] = true
				}
			}
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("audit row %s about the subject is missing from the export", id)
		}
	}
}

// The Leads section lists every lead the export reaches. Here that is a lead
// never promoted, holding an address left on a record merged into the subject.
// Mutation: match leads by the survivor's own addresses only, and it is missing.
func TestTheLeadsSectionListsALeadHoldingAMergedRecordsAddress(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	predecessor, priorLead, strangerLead := ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by, merged_into_id)
 VALUES (@id, 'Hedda Before', 'manual', 'system:privacy_test', @subject)`,
		pgx.StrictNamedArgs{"id": predecessor, "subject": subject.contact})
	mustExec(ctx, t, tx, `INSERT INTO contact_email (contact_id, email, source, captured_by)
 VALUES (@id, 'hedda.before@prior.example', 'manual', 'system:privacy_test')`,
		pgx.StrictNamedArgs{"id": predecessor})
	mustExec(ctx, t, tx, `INSERT INTO lead (id, full_name, email, source, captured_by) VALUES
 (@prior, 'Hedda Lead', 'hedda.before@prior.example', 'manual', 'system:privacy_test'),
 (@stranger, 'Someone Else', 'someone@else.example', 'manual', 'system:privacy_test')`,
		pgx.StrictNamedArgs{"prior": priorLead, "stranger": strangerLead})
	emails, leads, _, err := subjectReach(ctx, tx, subject.contact)
	if err != nil {
		t.Fatal(err)
	}
	var pkg SARPackage
	listed := map[string]bool{}
	for _, section := range sarRecordSections(&pkg, subject.contact, leads) {
		if section.dest != &pkg.Leads {
			continue
		}
		rows, err := rowMaps(ctx, tx, section.query, section.args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if id, ok := row["id"].(string); ok {
				listed[id] = true
			}
		}
	}
	if !listed[priorLead.String()] {
		t.Errorf("the lead holding %v is missing from the Leads section", emails)
	}
	if listed[strangerLead.String()] {
		t.Error("the Leads section lists a lead with no tie to the subject")
	}
}
