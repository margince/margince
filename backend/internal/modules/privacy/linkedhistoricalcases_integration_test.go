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

func TestLinkedCorrectionReachesIntroductionsAndDecidedApprovals(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	var requester ids.UUID
	if err := tx.QueryRow(ctx, `SELECT split_part(captured_by,':',2)::uuid FROM contact WHERE id=@subject`, pgx.StrictNamedArgs{"subject": subject.contact}).Scan(&requester); err != nil {
		t.Fatal(err)
	}
	introducer, intro, approval := ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO app_user(id,email,display_name) VALUES(@user,@email,'Introducer')`, pgx.StrictNamedArgs{"user": introducer, "email": introducer.String() + "@legacy.test"})
	mustExec(ctx, t, tx, `INSERT INTO intro_request(id,contact_id,requester_user_id,introducer_user_id,route_type,internal_reason,value_for_target,forwardable_note,status,closed_at,due_at,captured_by)
 VALUES(@id,@subject,@requester,@introducer,'direct',@marker,'','','cancelled',now(),timestamptz '2030-01-01','system:legacy')`, pgx.StrictNamedArgs{"id": intro, "subject": subject.contact, "requester": requester, "introducer": introducer, "marker": erasedMention})
	mustExec(ctx, t, tx, `INSERT INTO approval(id,kind,status,proposed_change,diff_hash,expires_at,proposed_by,summary,target_entity_type,target_entity_id,decided_at,consumed_at)
 VALUES(@id,'contact_update','approved','{}','legacy',timestamptz '2030-01-01','system:legacy','','contact',@subject,now(),now())`, pgx.StrictNamedArgs{"id": approval, "subject": subject.contact})
	introAudit := seedPersonalAudit(ctx, t, tx, "intro_request", intro)
	approvalAudit := seedPersonalAudit(ctx, t, tx, "approval", approval)
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	page, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed < 2 {
		t.Fatalf("changed %d linked entries, want at least two", page.Changed)
	}
	assertLegacyAuditImage(ctx, t, tx, introAudit, false)
	assertLegacyAuditImage(ctx, t, tx, approvalAudit, false)
	var verdict string
	if err := tx.QueryRow(ctx, `SELECT status FROM approval WHERE id=@id`, pgx.StrictNamedArgs{"id": approval}).Scan(&verdict); err != nil {
		t.Fatal(err)
	}
	if verdict != "approved" {
		t.Fatalf("changed the approval verdict to %q", verdict)
	}
}

func TestRequestIdentityIsKeptUntilOlderLinkedImagesAreCorrected(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	request := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO data_subject_request(id,kind,subject_ref,status,resolution,received_at,due_at,channel,receipt_reference)
 VALUES(@id,'access',@reference,'fulfilled','Private answer',now(),now()+interval '30 days','phone',@receipt)`, pgx.StrictNamedArgs{"id": request, "reference": subject.contact.String(), "receipt": "RCPT-" + request.String()})
	created, err := storekit.AuditWithEvidence(ctx, tx, "create", "data_subject_request", request, nil, map[string]any{"subject_ref": subject.contact.String(), "full_name": "Private Contact"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := storekit.AuditWithEvidence(ctx, tx, "update", "data_subject_request", request, map[string]any{"status": "in_progress"}, map[string]any{"full_name": "Private Contact"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	mustExec(ctx, t, tx, `UPDATE data_subject_request SET subject_ref='erased+'||id,resolution='New authorized answer' WHERE id=@id`, pgx.StrictNamedArgs{"id": request})
	later, err := storekit.AuditWithEvidence(ctx, tx, "update", "data_subject_request", request, map[string]any{"status": "in_progress"}, map[string]any{"full_name": "Private Contact"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	until := ids.NewV7()
	if _, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, until, 100, nil); err != nil {
		t.Fatal(err)
	}
	assertLegacyAuditImage(ctx, t, tx, created, true)
	assertLegacyAuditImage(ctx, t, tx, old, false)
	assertLegacyAuditImage(ctx, t, tx, later, true)
	if _, err := RedactLinkedHistoricalIdentitiesPageTx(ctx, tx, ids.UUID{}, until, 100, nil); err != nil {
		t.Fatal(err)
	}
	assertLegacyAuditImage(ctx, t, tx, created, false)
	assertLegacyAuditImage(ctx, t, tx, later, true)
}

func TestLinkedCorrectionKeepsDisputedRequestCopies(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	request := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO data_subject_request(id,kind,subject_ref,status,resolution,received_at,due_at,channel,receipt_reference,dispute_open)
 VALUES(@id,'access','erased+'||@id::uuid::text,'fulfilled','erased',now(),now()+interval '30 days','phone',@receipt,true)`, pgx.StrictNamedArgs{"id": request, "receipt": "RCPT-" + request.String()})
	original, err := storekit.AuditWithEvidence(ctx, tx, "create", "data_subject_request", request, nil, map[string]any{"subject_ref": subject.contact.String(), "full_name": "Private Contact"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	for _, step := range []auditPageForTest{RedactLinkedHistoricalAuditPageTx, RedactLinkedHistoricalIdentitiesPageTx} {
		page, err := step(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
		if err != nil {
			t.Fatal(err)
		}
		if page.Changed != 0 {
			t.Fatalf("changed %d disputed request entries", page.Changed)
		}
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

type auditPageForTest func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int, CountryReader) (HistoricalAuditPage, error)

func TestHistoricalEntryProofCannotRedactAnotherEntryOnTheSameRecord(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	record := ids.NewV7()
	original := seedPersonalAudit(ctx, t, tx, "intro_request", record)
	other := seedPersonalAudit(ctx, t, tx, "intro_request", record)
	boundary := legacyContactProof(ctx, t, tx, ids.NewV7())
	proof, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "audit_log", original, nil, map[string]any{"payload_redaction": true, "legacy_erased_record": true, "boundary": boundary.String()})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
	if err := storekit.RedactHistoricalAuditEntry(ctx, tx, proof); err != nil {
		t.Fatal(err)
	}
	assertLegacyAuditImage(ctx, t, tx, original, false)
	assertLegacyAuditImage(ctx, t, tx, other, true)
	if _, err := tx.Exec(ctx, `UPDATE audit_log SET before=NULL,after=NULL,evidence='{"redacted":true}' WHERE id=@other`, pgx.StrictNamedArgs{"other": other}); err == nil {
		t.Fatal("one entry's proof authorized a different audit entry")
	}
}

func TestTheLinkedMatcherDoesNotInferASubjectFromAnEditableWeeklyLabel(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	var owner ids.UUID
	if err := tx.QueryRow(ctx, `SELECT split_part(captured_by,':',2)::uuid FROM contact WHERE id=@subject`, pgx.StrictNamedArgs{"subject": subject.contact}).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	plan, commitment := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO weekly_plan(id,owner_id,local_week_start,captured_by) VALUES(@id,@owner,date '2026-11-02','system:legacy')`, pgx.StrictNamedArgs{"id": plan, "owner": owner})
	mustExec(ctx, t, tx, `INSERT INTO weekly_plan_commitment(id,plan_id,label,due_on,position,captured_by) VALUES(@id,@plan,@label,date '2026-11-06',0,'system:legacy')`, pgx.StrictNamedArgs{"id": commitment, "plan": plan, "label": erasedMention})
	original := seedPersonalAudit(ctx, t, tx, "weekly_plan_commitment", commitment)
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	page, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 0 {
		t.Fatalf("an editable label authorized %d redactions", page.Changed)
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

func TestDeletedBuyerCommentCopiesRespectTheDealHoldAndLaterHistory(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	pipeline, stage, deal, room, participant := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO pipeline(id,name,is_default,position) VALUES(@id,'Legacy room',false,99)`, pgx.StrictNamedArgs{"id": pipeline})
	mustExec(ctx, t, tx, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES(@id,@pipeline,'Open',0,'open',10)`, pgx.StrictNamedArgs{"id": stage, "pipeline": pipeline})
	mustExec(ctx, t, tx, `INSERT INTO deal(id,name,pipeline_id,stage_id,status,source,captured_by,legal_hold) VALUES(@id,'Legacy room',@pipeline,@stage,'open','manual','system:legacy',true)`, pgx.StrictNamedArgs{"id": deal, "pipeline": pipeline, "stage": stage})
	mustExec(ctx, t, tx, `INSERT INTO deal_room(id,deal_id,title,source,captured_by) VALUES(@id,@deal,'Legacy room','manual','system:legacy')`, pgx.StrictNamedArgs{"id": room, "deal": deal})
	mustExec(ctx, t, tx, `INSERT INTO deal_room_participant(id,room_id,full_name,email,source,captured_by,revoked_at) VALUES(@id,@room,'Erased',@email,'manual','system:legacy',now())`, pgx.StrictNamedArgs{"id": participant, "room": room, "email": erasedEmail})
	buyer := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:" + participant.String()})
	original := seedPersonalAudit(buyer, t, tx, "deal_room_comment", ids.NewV7())
	seller := seedPersonalAudit(ctx, t, tx, "deal_room_comment", ids.NewV7())
	if _, err := storekit.AuditWithEvidence(ctx, tx, "erase", "deal_room_participant", participant, nil, nil, map[string]any{"cause": "contact_erasure"}); err != nil {
		t.Fatal(err)
	}
	later := seedPersonalAudit(buyer, t, tx, "deal_room_comment", ids.NewV7())
	mustExec(ctx, t, tx, `INSERT INTO event_outbox(stream,envelope) VALUES('gw:events:crm:deal_room',jsonb_build_object('entity',jsonb_build_object('type','deal_room','id',@room::uuid::text),'trace',jsonb_build_object('audit_log_id',@audit::uuid::text),'payload',jsonb_build_object('author_name','Private Contact')))`, pgx.StrictNamedArgs{"room": room, "audit": original})
	page, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 0 {
		t.Fatalf("changed %d held room entries", page.Changed)
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
	mustExec(ctx, t, tx, `UPDATE deal SET legal_hold=false WHERE id=@deal`, pgx.StrictNamedArgs{"deal": deal})
	if _, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil); err != nil {
		t.Fatal(err)
	}
	assertLegacyAuditImage(ctx, t, tx, original, false)
	assertLegacyAuditImage(ctx, t, tx, seller, true)
	assertLegacyAuditImage(ctx, t, tx, later, true)
	var retained int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=@audit::uuid::text`, pgx.StrictNamedArgs{"audit": original}).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 0 {
		t.Fatalf("kept %d comment event copies", retained)
	}
}
