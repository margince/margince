// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// These rows reproduce the old shape: an import lost its message identity,
// and another arrival then stored the mailbox copy under its seat's key.
func TestTheMailCopiesTaskKeepsFilesLinksReadStateAndHoldsOnce(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	kept, imported, scoped := ids.NewV7(), ids.NewV7(), ids.NewV7()
	const message = "duplicate@mail-task.test"
	for _, row := range []struct {
		id                 ids.UUID
		system, key, stamp string
	}{
		{kept, "email", message, "connector:gmail:" + e.Rep1.String()},
		{imported, "oldcrm", "engagement-1", "human:" + e.Rep1.String()},
		{scoped, "email", connector.SeatScopedMailKey(message, e.Rep1.String()), "connector:gmail:" + e.Rep1.String()},
	} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,body,occurred_at,source,source_system,source_id,captured_by,raw) VALUES($1,'email','Review','Mailbox reading','2026-10-01T09:00:00Z','manual',$2,$3,$4,jsonb_build_object('rfc_message_id',$5::text))`, row.id, row.system, row.key, row.stamp, message)
	}
	e.WsExec(t, `INSERT INTO activity_identity(identity_kind,identity_key,activity_id,attested_by) VALUES('mail',$1,$2,'connector:gmail')`, message, kept)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,posture_at_import,verdict_status,provider_message_ids) VALUES($1,$2,'held','held',ARRAY['copy-provider-id'])`, imported, e.Rep1)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,posture_at_import,verdict_status,provider_message_ids) VALUES($1,$2,'held','held',ARRAY['kept-provider-id'])`, kept, e.Rep1)
	e.WsExec(t, `INSERT INTO activity_reader_state(activity_id,reader_id,state,set_by) VALUES($1,$2,'handled',$3)`, imported, e.Rep1, "human:"+e.Rep1.String())
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Mail participant", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](imported), activities.RelinkActivityInput{EntityType: "contact", EntityID: ids.UUID(contact.Id)}); err != nil {
		t.Fatal(err)
	}
	file := ids.NewV7()
	e.WsExec(t, `INSERT INTO attachment(id,entity_type,entity_id,activity_id,filename,content_type,byte_size,storage_key,source,captured_by) VALUES($1,'activity',$2,$2,'proposal.pdf','application/pdf',12,'test/proposal.pdf','manual',$3)`, file, imported, "human:"+e.Rep1.String())
	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, mailCopiesDataTask(e.Pool))
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=ANY($1) AND archived_at IS NULL`, []ids.UUID{kept, imported, scoped}); got != 1 {
		t.Fatalf("live mail copies=%d, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND contact_id=$2`, kept, contact.Id); got != 1 {
		t.Fatal("the imported record link was lost")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id=$1 AND activity_id=$2 AND archived_at IS NULL`, file, kept); got != 1 {
		t.Fatal("the file did not follow the surviving message")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_reader_state WHERE activity_id=$1 AND reader_id=$2 AND state='handled'`, kept, e.Rep1); got != 1 {
		t.Fatal("the reader's handled state was lost")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND user_id=$2 AND verdict_status='held'`, kept, e.Rep1); got != 1 {
		t.Fatal("the sharing hold was lost")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND provider_message_ids @> ARRAY['copy-provider-id','kept-provider-id']`, kept); n != 1 {
		t.Fatal("provider identities were lost")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=ANY($1) AND action='archive'`, []ids.UUID{imported, scoped}); got != 2 {
		t.Fatalf("archive audits=%d, want 2 across both runs", got)
	}
	if err := undoEntry(t, e, entityTypeActivity, imported, latestAuditRowID(t, e, entityTypeActivity, imported, actionArchive)); refusedFor(t, err) != ReasonNotRestorableByThisPath {
		t.Fatalf("folded mail could be restored independently: %v", err)
	}
}

func TestMailRepairPreservesCopiesReferencedByTasks(t *testing.T) {
	e := integration.Setup(t)
	kept, duplicate, task := ids.NewV7(), ids.NewV7(), ids.NewV7()
	const message = "referenced-copy@mail-task.test"
	for _, id := range []ids.UUID{kept, duplicate} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by,raw) VALUES($1,'email','Question','2026-10-01T09:00:00Z','manual',$2,jsonb_build_object('rfc_message_id',$3::text))`, id, "human:"+e.Rep1.String(), message)
	}
	e.WsExec(t, `INSERT INTO activity_identity(identity_kind,identity_key,activity_id,attested_by) VALUES('mail',$1,$2,'connector:gmail')`, message, kept)
	e.WsExec(t, `INSERT INTO activity(id,kind,subject,source,source_activity_id,captured_by) VALUES($1,'task','Answer','manual',$2,$3)`, task, duplicate, "human:"+e.Rep1.String())
	runCalendarDescriptionTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, mailCopiesDataTask(e.Pool))
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=ANY($1) AND archived_at IS NULL`, []ids.UUID{kept, duplicate}); got != 2 {
		t.Fatalf("referenced mail copies=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND source_activity_id=$2`, task, duplicate); got != 1 {
		t.Fatal("task source changed")
	}
}
