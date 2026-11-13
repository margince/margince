// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type inquiryClaimFixture struct {
	admin, rep *apptest.AppEnv
	recipient  crmcontracts.User
	tasks      []crmcontracts.Activity
}

func setupInquiryClaim(t *testing.T) inquiryClaimFixture {
	t.Helper()
	e := setupFormsApp(t)
	e.DescribeCompany(t)
	f := inquiryClaimFixture{admin: e}
	if status := e.Call(t, "POST", "/v1/users", integration.AnyMap{"email": "taker@inquiry.example", "display_name": "Inquiry colleague", "role": "rep"}, nil, &f.recipient); status != http.StatusCreated {
		t.Fatalf("invite taker: %d", status)
	}
	apptest.SetPasswordByLink(t, e, f.recipient.Id.String(), "Inquiry-taker-2026!")
	f.rep = e.AsSecondSeat(t, "taker@inquiry.example", "Inquiry-taker-2026!")
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	setupInquiryPolicy(t, e, form, "coordinator")
	engine := compose.NewWorkflowEngine(e.DB())
	for range 2 {
		answer := browserPost(t, e, form.FormKey, url.Values{"email": {"claim@visitor.example"}, "message": {"Please call"}, "_mrg_id": {ids.NewV7().String()}}, siteOrigin)
		if answer.Status != http.StatusAccepted {
			t.Fatalf("inquiry: %d", answer.Status)
		}
		var notes crmcontracts.ActivityListResponse
		if status := e.Call(t, "GET", "/v1/activities?website_form_id="+form.ID+"&limit=1", nil, nil, &notes); status != http.StatusOK || len(notes.Data) != 1 {
			t.Fatalf("accepted note: %d %+v", status, notes)
		}
		if err := engine.HandleEvent(t.Context(), inquiryEventByID(t, e, notes.Data[0].Id, "activity.captured")); err != nil {
			t.Fatal(err)
		}
		var tasks crmcontracts.ActivityListResponse
		if status := e.Call(t, "GET", "/v1/activities?source_activity_id="+notes.Data[0].Id.String(), nil, nil, &tasks); status != http.StatusOK || len(tasks.Data) != 1 {
			t.Fatalf("inquiry task: %d %+v", status, tasks)
		}
		f.tasks = append(f.tasks, tasks.Data[0])
	}
	return f
}

func (f inquiryClaimFixture) preview(t *testing.T) crmcontracts.WebsiteInquiryAssignment {
	t.Helper()
	var out crmcontracts.WebsiteInquiryAssignment
	body := integration.AnyMap{"mode": "take", "recipient_id": f.recipient.Id}
	if status := f.rep.Call(t, "POST", "/v1/activities/"+f.tasks[0].Id.String()+"/inquiry-assignment/preview", body, nil, &out); status != http.StatusOK {
		t.Fatalf("preview take: %d %+v", status, out)
	}
	if !out.ChangesRecordOwner || out.RecordOwnerId != nil || out.Task.Version == nil || out.RecordVersion == nil {
		t.Fatalf("take preview did not name the unowned lead and coordinator task: %+v", out)
	}
	return out
}

func (f inquiryClaimFixture) take(t *testing.T, preview crmcontracts.WebsiteInquiryAssignment) int {
	t.Helper()
	return applyInquiryAssignment(t, f.rep, "take", preview)
}

func applyInquiryAssignment(t *testing.T, seat *apptest.AppEnv, mode string, preview crmcontracts.WebsiteInquiryAssignment) int {
	t.Helper()
	body := integration.AnyMap{"mode": mode, "recipient_id": preview.RecipientId, "record_type": preview.RecordType, "record_id": preview.RecordId, "record_version": preview.RecordVersion}
	return seat.Call(t, "POST", "/v1/activities/"+preview.Task.Id.String()+"/inquiry-assignment", body, map[string]string{"If-Match": strconv.FormatInt(int64(*preview.Task.Version), 10)}, nil)
}

func TestTakingAnInquiryClaimsItsLeadAndOnlyItsTask(t *testing.T) {
	f := setupInquiryClaim(t)
	var work crmcontracts.Worklist
	if status := f.rep.Call(t, "GET", "/v1/worklist?scope=all&view=today&filter=website_inquiries", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("rep gained everyone else's queue: %d", status)
	}
	if status := f.admin.Call(t, "GET", "/v1/worklist?view=today&filter=website_inquiries", nil, nil, &work); status != http.StatusOK {
		t.Fatalf("coordinator inquiry work: %d", status)
	}
	offered := false
	for _, row := range work.Queue {
		if row.Id == f.tasks[0].Id.String() {
			for _, action := range row.Actions {
				offered = offered || action == "take"
			}
		}
	}
	if !offered {
		t.Fatalf("visible coordinator inquiry did not offer an atomic claim: %+v", work.Queue)
	}
	preview := f.preview(t)
	if status := f.take(t, preview); status != http.StatusOK {
		t.Fatalf("take inquiry: %d", status)
	}
	var owner, holder ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id FROM lead l CROSS JOIN activity a WHERE l.id=@lead AND a.id=@task`, pgx.NamedArgs{"lead": preview.RecordId, "task": f.tasks[0].Id}).Scan(&owner, &holder); err != nil {
		t.Fatal(err)
	}
	if owner != ids.UUID(f.recipient.Id) || holder != owner {
		t.Fatalf("claim split lead and task: owner=%s holder=%s", owner, holder)
	}
	event := inquiryEventByID(t, f.admin, preview.RecordId, "lead.updated")
	if err := compose.NewWorkflowEngine(f.admin.DB()).HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var other ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT assignee_id FROM activity WHERE id=@id`, pgx.NamedArgs{"id": f.tasks[1].Id}).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if f.tasks[1].AssigneeId == nil || other != ids.UUID(*f.tasks[1].AssigneeId) {
		t.Fatalf("taking one inquiry moved another task to %s", other)
	}
}

func TestAStaleInquiryTaskDoesNotClaimItsLead(t *testing.T) {
	f := setupInquiryClaim(t)
	preview := f.preview(t)
	if status := f.admin.Call(t, "PATCH", "/v1/activities/"+f.tasks[0].Id.String(), integration.AnyMap{"subject": "Review this inquiry first"}, map[string]string{"If-Match": strconv.FormatInt(int64(*preview.Task.Version), 10)}, nil); status != http.StatusOK {
		t.Fatalf("change task: %d", status)
	}
	if status := f.take(t, preview); status != http.StatusConflict {
		t.Fatalf("stale take: %d", status)
	}
	var owner *ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT owner_id FROM lead WHERE id=@id`, pgx.NamedArgs{"id": preview.RecordId}).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		t.Fatalf("stale task still claimed the lead for %s", owner)
	}
}

func TestAFailedInquiryTaskMoveRollsBackItsLeadClaim(t *testing.T) {
	f := setupInquiryClaim(t)
	preview := f.preview(t)
	if _, err := f.admin.Owner.Exec(t.Context(), `CREATE FUNCTION refuse_inquiry_move() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected task move failure'; END $$;
 CREATE TRIGGER refuse_inquiry_move BEFORE UPDATE OF assignee_id ON activity FOR EACH ROW WHEN (OLD.assignee_id IS DISTINCT FROM NEW.assignee_id) EXECUTE FUNCTION refuse_inquiry_move()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.admin.Owner.Exec(context.Background(), `DROP TRIGGER refuse_inquiry_move ON activity; DROP FUNCTION refuse_inquiry_move()`); err != nil {
			t.Error(err)
		}
	})
	var auditBefore, eventBefore int
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM audit_log),(SELECT count(*) FROM event_outbox)`).Scan(&auditBefore, &eventBefore); err != nil {
		t.Fatal(err)
	}
	if status := f.take(t, preview); status != http.StatusInternalServerError {
		t.Fatalf("injected task failure: %d", status)
	}
	var owner *ids.UUID
	var auditAfter, eventAfter int
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT owner_id,(SELECT count(*) FROM audit_log),(SELECT count(*) FROM event_outbox) FROM lead WHERE id=@id`, pgx.NamedArgs{"id": preview.RecordId}).Scan(&owner, &auditAfter, &eventAfter); err != nil {
		t.Fatal(err)
	}
	if owner != nil || auditAfter != auditBefore || eventAfter != eventBefore {
		t.Fatalf("failed task committed part of its claim: owner=%v audit=%d/%d events=%d/%d", owner, auditBefore, auditAfter, eventBefore, eventAfter)
	}
}

func TestConcurrentInquiryClaimsHaveOneWinner(t *testing.T) {
	f := setupInquiryClaim(t)
	first := f.preview(t)
	var other crmcontracts.User
	if status := f.admin.Call(t, "POST", "/v1/users", integration.AnyMap{"email": "other-taker@inquiry.example", "display_name": "Other colleague", "role": "rep"}, nil, &other); status != http.StatusCreated {
		t.Fatalf("invite other taker: %d", status)
	}
	apptest.SetPasswordByLink(t, f.admin, other.Id.String(), "Inquiry-taker-2026!")
	secondSeat := f.admin.AsSecondSeat(t, "other-taker@inquiry.example", "Inquiry-taker-2026!")
	var second crmcontracts.WebsiteInquiryAssignment
	if status := secondSeat.Call(t, "POST", "/v1/activities/"+f.tasks[0].Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "take", "recipient_id": other.Id}, nil, &second); status != http.StatusOK || second.RecordVersion == nil || second.Task.Version == nil {
		t.Fatalf("other preview: %d %+v", status, second)
	}
	if first.RecordId != second.RecordId || *first.RecordVersion != *second.RecordVersion || *first.Task.Version != *second.Task.Version {
		t.Fatal("claims did not start from the same versions")
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	statuses := make([]int, 2)
	wait.Add(2)
	go func() { defer wait.Done(); <-start; statuses[0] = f.take(t, first) }()
	go func() {
		defer wait.Done()
		<-start
		statuses[1] = applyInquiryAssignment(t, secondSeat, "take", second)
	}()
	close(start)
	wait.Wait()
	winners := 0
	for _, status := range statuses {
		if status == http.StatusOK {
			winners++
		} else if status != http.StatusConflict && status != http.StatusForbidden {
			t.Fatalf("unexpected competing claim result: %v", statuses)
		}
	}
	if winners != 1 {
		t.Fatalf("competing claims returned %v", statuses)
	}
	var owner, holder ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id FROM lead l CROSS JOIN activity a WHERE l.id=@lead AND a.id=@task`, pgx.NamedArgs{"lead": first.RecordId, "task": first.Task.Id}).Scan(&owner, &holder); err != nil {
		t.Fatal(err)
	}
	if owner != holder || (owner != ids.UUID(f.recipient.Id) && owner != ids.UUID(other.Id)) {
		t.Fatalf("competing claims split lead and task: owner=%s holder=%s", owner, holder)
	}
}

func TestACoordinatorCanTakeTheLeadOfTheirOwnInquiryTask(t *testing.T) {
	f := setupInquiryClaim(t)
	actor := apptest.SessionUserID(t, f.admin)
	var preview crmcontracts.WebsiteInquiryAssignment
	if status := f.admin.Call(t, "POST", "/v1/activities/"+f.tasks[0].Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "take", "recipient_id": actor}, nil, &preview); status != http.StatusOK || !preview.ChangesRecordOwner {
		t.Fatalf("coordinator review: %d %+v", status, preview)
	}
	if status := applyInquiryAssignment(t, f.admin, "take", preview); status != http.StatusOK {
		t.Fatalf("coordinator take: %d", status)
	}
	var owner, holder ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id FROM lead l CROSS JOIN activity a WHERE l.id=@lead AND a.id=@task`, pgx.NamedArgs{"lead": preview.RecordId, "task": preview.Task.Id}).Scan(&owner, &holder); err != nil {
		t.Fatal(err)
	}
	if owner.String() != actor || holder != owner {
		t.Fatalf("coordinator claim left owner=%s holder=%s", owner, holder)
	}
}

func TestAssigningAndFollowingUpMovesOnlyTheReviewedInquiry(t *testing.T) {
	f := setupInquiryClaim(t)
	var preview crmcontracts.WebsiteInquiryAssignment
	path := "/v1/activities/" + f.tasks[0].Id.String() + "/inquiry-assignment"
	if status := f.admin.Call(t, "POST", path+"/preview", integration.AnyMap{"mode": "assign_and_follow_up", "recipient_id": f.recipient.Id}, nil, &preview); status != http.StatusOK || !preview.ChangesRecordOwner {
		t.Fatalf("review handover: %d %+v", status, preview)
	}
	if status := applyInquiryAssignment(t, f.admin, "assign_and_follow_up", preview); status != http.StatusOK {
		t.Fatalf("assign and follow up: %d", status)
	}
	if err := compose.NewWorkflowEngine(f.admin.DB()).HandleEvent(t.Context(), inquiryEventByID(t, f.admin, preview.RecordId, "lead.updated")); err != nil {
		t.Fatal(err)
	}
	var owner, holder, other ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id,b.assignee_id FROM lead l CROSS JOIN activity a CROSS JOIN activity b WHERE l.id=@lead AND a.id=@task AND b.id=@other`, pgx.NamedArgs{"lead": preview.RecordId, "task": preview.Task.Id, "other": f.tasks[1].Id}).Scan(&owner, &holder, &other); err != nil {
		t.Fatal(err)
	}
	if owner != ids.UUID(f.recipient.Id) || holder != owner || f.tasks[1].AssigneeId == nil || other != ids.UUID(*f.tasks[1].AssigneeId) {
		t.Fatalf("handover changed the wrong work: owner=%s holder=%s other=%s", owner, holder, other)
	}
}

func TestTakingAContactInquiryKeepsTheContactOwner(t *testing.T) {
	f := setupInquiryRecipient(t)
	engine := compose.NewWorkflowEngine(f.e.DB())
	if err := engine.HandleEvent(t.Context(), f.event); err != nil {
		t.Fatal(err)
	}
	var lead ids.UUID
	var version int64
	if err := f.e.Owner.QueryRow(t.Context(), `SELECT l.id,l.version FROM lead l JOIN activity_link a ON a.lead_id=l.id WHERE a.activity_id=@id`, pgx.NamedArgs{"id": f.note.Id}).Scan(&lead, &version); err != nil {
		t.Fatal(err)
	}
	var promoted crmcontracts.PromoteLeadResponse
	if status := f.e.Call(t, "POST", "/v1/leads/"+lead.String()+"/promote", integration.AnyMap{"trigger": "human_qualify"}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, &promoted); status != http.StatusOK {
		t.Fatalf("promote inquiry lead: %d", status)
	}
	if err := engine.HandleEvent(t.Context(), inquiryEventByID(t, f.e, crmcontracts.Id(lead), "lead.promoted")); err != nil {
		t.Fatal(err)
	}
	var tasks crmcontracts.ActivityListResponse
	if status := f.e.Call(t, "GET", "/v1/activities?source_activity_id="+f.note.Id.String(), nil, nil, &tasks); status != http.StatusOK || len(tasks.Data) != 1 {
		t.Fatalf("contact inquiry task: %d %+v", status, tasks)
	}
	actor := apptest.SessionUserID(t, f.e)
	var preview crmcontracts.WebsiteInquiryAssignment
	if status := f.e.Call(t, "POST", "/v1/activities/"+tasks.Data[0].Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "take", "recipient_id": actor}, nil, &preview); status != http.StatusOK {
		t.Fatalf("contact task preview: %d %+v", status, preview)
	}
	if preview.RecordType != "contact" || preview.RecordId != promoted.Contact.Id || preview.ChangesRecordOwner || preview.RecordOwnerId == nil || *preview.RecordOwnerId != f.colleague.Id {
		t.Fatalf("taking the task proposed a contact handover: %+v", preview)
	}
	if status := applyInquiryAssignment(t, f.e, "take", preview); status != http.StatusOK {
		t.Fatalf("take contact inquiry: %d", status)
	}
	var owner, holder ids.UUID
	if err := f.e.Owner.QueryRow(t.Context(), `SELECT c.owner_id,a.assignee_id FROM contact c CROSS JOIN activity a WHERE c.id=@contact AND a.id=@task`, pgx.NamedArgs{"contact": promoted.Contact.Id, "task": tasks.Data[0].Id}).Scan(&owner, &holder); err != nil {
		t.Fatal(err)
	}
	if owner != ids.UUID(f.colleague.Id) || holder.String() != actor {
		t.Fatalf("task claim changed contact responsibility: owner=%s holder=%s", owner, holder)
	}
}

func TestAnInquiryClaimRejectsAnotherRecordAtTheSameVersion(t *testing.T) {
	f := setupInquiryClaim(t)
	preview := f.preview(t)
	preview.RecordId = crmcontracts.Id(ids.NewV7())
	if status := f.take(t, preview); status != http.StatusConflict {
		t.Fatalf("claim accepted the wrong reviewed record: %d", status)
	}
	var count int
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT count(*) FROM lead WHERE owner_id IS NOT NULL AND email='claim@visitor.example'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("wrong record identity still claimed the inquiry lead")
	}
}

func TestAReadShareOnAnInquiryLeadDoesNotHandItOver(t *testing.T) {
	f := setupInquiryClaim(t)
	admin := apptest.SessionUserID(t, f.admin)
	var taken crmcontracts.WebsiteInquiryAssignment
	if status := f.admin.Call(t, "POST", "/v1/activities/"+f.tasks[0].Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "take", "recipient_id": admin}, nil, &taken); status != http.StatusOK {
		t.Fatalf("admin take preview: %d", status)
	}
	if status := applyInquiryAssignment(t, f.admin, "take", taken); status != http.StatusOK {
		t.Fatalf("admin take: %d", status)
	}
	if status := f.admin.Call(t, "POST", "/v1/record-grants", integration.AnyMap{
		"record_type": "lead", "record_id": taken.RecordId, "subject_type": "user", "subject_id": f.recipient.Id, "access": "read",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("read share: %d", status)
	}
	var held crmcontracts.Activity
	if status := f.admin.Call(t, "GET", "/v1/activities/"+f.tasks[0].Id.String(), nil, nil, &held); status != http.StatusOK || held.Version == nil {
		t.Fatalf("held task: %d", status)
	}
	var lead crmcontracts.Lead
	if status := f.rep.Call(t, "GET", "/v1/leads/"+taken.RecordId.String(), nil, nil, &lead); status != http.StatusOK || lead.Version == nil {
		t.Fatalf("the read share does not let the colleague see the lead: %d", status)
	}
	handOver := taken
	handOver.Task = held
	handOver.RecipientId = f.recipient.Id
	handOver.RecordVersion = lead.Version
	if status := f.rep.Call(t, "POST", "/v1/activities/"+held.Id.String()+"/inquiry-assignment/preview", integration.AnyMap{"mode": "assign_and_follow_up", "recipient_id": f.recipient.Id}, nil, nil); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("a read share previewed a hand-over: %d", status)
	}
	if status := applyInquiryAssignment(t, f.rep, "assign_and_follow_up", handOver); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("a read share handed the lead over: %d", status)
	}
	var owner, holder ids.UUID
	if err := f.admin.Owner.QueryRow(t.Context(), `SELECT l.owner_id,a.assignee_id FROM lead l CROSS JOIN activity a WHERE l.id=@lead AND a.id=@task`, pgx.NamedArgs{"lead": taken.RecordId, "task": held.Id}).Scan(&owner, &holder); err != nil {
		t.Fatal(err)
	}
	if owner.String() != admin || holder.String() != admin {
		t.Fatalf("the refused hand-over moved the lead or its task: owner=%s holder=%s", owner, holder)
	}
}
