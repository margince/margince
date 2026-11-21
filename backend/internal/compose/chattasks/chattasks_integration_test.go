// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chattasks_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/chattasks"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	bookedLine = "Lena Hoffmann: I booked a slot for Thursday."
	bookedChat = bookedLine + " Me: Great, see you then."
	laterLine  = "Lena Hoffmann: Please ping me again next week."
)

// imageReader is the document lane that reads the screenshot: it carries images.
type imageReader struct{ *ai.FakeClient }

func (imageReader) AttachmentMIMEs() []string { return []string{"image/*"} }

func screenshotReply(text string) string {
	return `{"fields":[` +
		`{"field":"chat_text","stated":"stated","value":"` + text + `","source_quote":"` + text + `","page_or_section":"messages","confidence":0.95},` +
		`{"field":"counterparty_name","stated":"stated","value":"Lena Hoffmann","source_quote":"Lena Hoffmann","page_or_section":"header","confidence":0.95},` +
		`{"field":"counterparty_phone","stated":"not_stated","value":"","source_quote":"","page_or_section":"","confidence":0}]}`
}

func taskReply(action, dueOn, quote string) string {
	return `{"results":[{"id":"task1","action":"` + action + `","due_on":"` + dueOn + `","quote":"` + quote + `","confidence":0.95}]}`
}

// chatOnRecord is a screenshot read for a chat on a record that holds one
// open task of the admin's, made before the chat is filed.
type chatOnRecord struct {
	env        *integration.Env
	record     string
	recordID   ids.UUID
	attachment ids.UUID
	reading    ids.UUID
	task       ids.UUID
	due        time.Time
}

func seedRecord(t *testing.T, env *integration.Env, record string) ids.UUID {
	t.Helper()
	switch record {
	case "lead":
		lead, _, err := env.Contacts.CreateLead(env.Admin(), contacts.CreateLeadInput{FullName: new("Lena Hoffmann"), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(lead.Id)
	case "deal":
		pipeline, open, _ := integration.DealFixture(t, env)
		return env.SeedDeal(t, "Hoffmann rollout", pipeline, open, &env.AdminUser)
	default:
		return env.SeedContact(t, "Lena Hoffmann", &env.AdminUser)
	}
}

func readChatOn(t *testing.T, record, text string) *chatOnRecord {
	t.Helper()
	env := integration.Setup(t)
	f := &chatOnRecord{env: env, record: record, recordID: seedRecord(t, env, record)}
	f.readScreenshot(t, text)
	f.due = clocktest.Now(t).AddDate(0, 0, 1).UTC().Truncate(time.Second)
	f.task = f.openTask(env.Admin(), t, "Follow up on the meeting request", record, f.recordID)
	return f
}

// readScreenshot uploads one more screenshot on the record and reads it.
func (f *chatOnRecord) readScreenshot(t *testing.T, text string) {
	t.Helper()
	ctx := f.env.Admin()
	store := activities.NewStore(f.env.DB()).WithBlobstore(blobstore.NewMemory())
	var screenshot bytes.Buffer
	if err := png.Encode(&screenshot, image.NewNRGBA(image.Rect(0, 0, 300, 600))); err != nil {
		t.Fatal(err)
	}
	att, err := store.UploadAttachment(ctx, activities.AttachmentInput{
		EntityType: f.record, EntityID: f.recordID, Filename: "chat.png", ContentType: "image/png",
		Content: bytes.NewReader(screenshot.Bytes()),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.attachment = ids.UUID(att.Id)
	reading, _, err := store.StartExtractionReadFor(ctx, f.attachment, activities.ExtractionQuestionChatCapture, "human:"+f.env.AdminUser.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	f.reading = reading.ID
	reader := compose.NewDocumentExtractor(f.env.Pool, imageReader{ai.NewFakeClient().Script(screenshotReply(text))}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := reader.Read(ctx, store, f.reading, f.attachment); err != nil {
		t.Fatal(err)
	}
}

func (f *chatOnRecord) openTask(ctx context.Context, t *testing.T, subject, record string, on ids.UUID) ids.UUID {
	t.Helper()
	source := "manual"
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalSystem {
		source = "system"
	}
	task, _, err := f.env.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: new(subject), Source: source, DueAt: &f.due,
		AssigneeID: new(ids.From[ids.UserKind](f.env.AdminUser)),
		Links:      []activities.ActivityLinkInput{{EntityType: record, EntityID: on}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id)
}

func (f *chatOnRecord) file(t *testing.T, lane ai.Completer, body string) ids.UUID {
	t.Helper()
	confirm := true
	out, err := compose.NewChatCaptureFile(f.env.Pool, lane).File(f.env.Admin(), f.attachment, crmcontracts.FileChatCaptureRequest{
		ExtractionId: openapi_types.UUID(f.reading), ChannelProvider: "whatsapp", Body: &body, ConfirmOtherCounterparty: &confirm,
	})
	if err != nil {
		t.Fatalf("filing the chat on the %s: %v", f.record, err)
	}
	return ids.UUID(out.ActivityId)
}

func (f *chatOnRecord) taskNow(t *testing.T, id ids.UUID) crmcontracts.Activity {
	t.Helper()
	task, err := f.env.Activities.GetActivity(f.env.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func (f *chatOnRecord) receipt(t *testing.T) activities.TaskCompletion {
	t.Helper()
	got, err := f.env.Activities.RecentTaskCompletions(f.env.Admin(), time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("Done for you holds %d task receipts, want 1: %v", len(got), err)
	}
	return got[0]
}

func (f *chatOnRecord) undo(t *testing.T, receipt activities.TaskCompletion) {
	t.Helper()
	seam := compose.NewRestoreSeam(f.env.Pool, compose.NewProvider(f.env.Pool), deals.NewStore(f.env.DB(), compose.DealsInstallation()))
	if _, err := seam.Restore(f.env.Admin(), "activity", receipt.ID, receipt.AuditID, receipt.Version); err != nil {
		t.Fatalf("Undo: %v", err)
	}
}

func TestAChatThatSaysBookedClosesTheTaskWithAReceiptAndUndoHolds(t *testing.T) {
	for _, record := range []string{"contact", "lead", "deal"} {
		t.Run(record, func(t *testing.T) {
			f := readChatOn(t, record, bookedChat)
			chat := f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
			if n := f.env.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND `+record+`_id = $2`, chat, f.recordID); n != 1 {
				t.Fatalf("the chat is not filed on the %s", record)
			}
			if done := f.taskNow(t, f.task).IsDone; done == nil || !*done {
				t.Fatal("a chat saying the slot is booked left the task open")
			}
			receipt := f.receipt(t)
			if !receipt.Chat || receipt.Moved || receipt.Quote != bookedLine || receipt.EmailID != chat {
				t.Fatalf("the receipt does not name the chat line it rests on: %+v", receipt)
			}
			f.undo(t, receipt)
			f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
			if done := f.taskNow(t, f.task).IsDone; done == nil || *done {
				t.Fatal("the same chat closed the task again after Undo")
			}
		})
	}
}

func TestAChatThatSaysNextWeekMovesTheDueDateAndUndoPutsItBack(t *testing.T) {
	f := readChatOn(t, "contact", laterLine)
	next := clocktest.Now(t).AddDate(0, 0, 7).UTC().Format(time.DateOnly)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionNewDate, next, laterLine)), laterLine)
	moved := f.taskNow(t, f.task)
	if moved.IsDone == nil || *moved.IsDone || moved.DueAt == nil || moved.DueAt.Format(time.DateOnly) != next {
		t.Fatalf("the task did not move to %s and stay open: %+v", next, moved)
	}
	receipt := f.receipt(t)
	if !receipt.Moved || receipt.Quote != laterLine {
		t.Fatalf("the receipt does not say the task moved on the chat's line: %+v", receipt)
	}
	f.undo(t, receipt)
	if back := f.taskNow(t, f.task); back.DueAt == nil || !back.DueAt.Equal(f.due) {
		t.Fatalf("Undo did not put the due date back to %s: %v", f.due, back.DueAt)
	}
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionNewDate, next, laterLine)), laterLine)
	if again := f.taskNow(t, f.task); again.DueAt == nil || !again.DueAt.Equal(f.due) {
		t.Fatalf("the same chat moved the task again after Undo: %v", again.DueAt)
	}
}

func TestAChatChangesNoTaskItDoesNotSettle(t *testing.T) {
	for name, lane := range map[string]ai.Completer{
		"the reading says nothing": ai.NewFakeClient().Script(taskReply(chattasks.ActionNone, "", "")),
		"the reading is unsure": ai.NewFakeClient().Script(`{"results":[{"id":"task1","action":"done","due_on":"","quote":"` +
			bookedLine + `","confidence":0.4}]}`),
		"the quote is not in the chat": ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", "Lena Hoffmann: signed.")),
		"no model is set up":           nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := readChatOn(t, "contact", bookedChat)
			f.file(t, lane, bookedChat)
			if task := f.taskNow(t, f.task); task.IsDone == nil || *task.IsDone || task.DueAt == nil || !task.DueAt.Equal(f.due) {
				t.Fatalf("the task changed: %+v", task)
			}
		})
	}
}

// Only the filer's own open tasks on the chat's record are read. A task on
// another contact, or a colleague's, is never offered to the model.
func TestAChatReadsOnlyTheFilersTasksOnItsRecord(t *testing.T) {
	f := readChatOn(t, "contact", bookedChat)
	elsewhere := f.openTask(f.env.Admin(), t, "Follow up on the meeting request", "contact", f.env.SeedContact(t, "Jonas Reiter", &f.env.AdminUser))
	margince := f.openTask(principal.SystemActing(f.env.Admin(), "system:follow-up-test"), t, "Margince follow-up", "contact", f.recordID)
	colleague, _, err := f.env.Activities.LogActivity(f.env.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Colleague follow-up"), Source: "manual", DueAt: &f.due,
		AssigneeID: new(ids.From[ids.UserKind](f.env.Rep1)),
		Links:      []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.recordID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lane := ai.NewFakeClient().Script(`{"results":[{"id":"task1","action":"done","due_on":"","quote":"` + bookedLine +
		`","confidence":0.95},{"id":"task2","action":"done","due_on":"","quote":"` + bookedLine + `","confidence":0.95}]}`)
	f.file(t, lane, bookedChat)
	if done := f.taskNow(t, f.task).IsDone; done == nil || !*done {
		t.Fatal("the filer's task on the chat's record stayed open")
	}
	for label, id := range map[string]ids.UUID{"another contact's": elsewhere, "a colleague's": ids.UUID(colleague.Id), "Margince's own": margince} {
		if done := f.taskNow(t, id).IsDone; done == nil || *done {
			t.Errorf("%s task was closed by the chat", label)
		}
	}
	calls := lane.Calls()
	if len(calls) != 1 || !strings.Contains(string(calls[0].Payload), "task1") || strings.Contains(string(calls[0].Payload), "task2") {
		t.Fatalf("the model was not asked about the filer's one task alone: %d calls", len(calls))
	}
}

// A task the user opened again after an automatic answer stays theirs: a
// later chat does not close it.
func TestALaterChatLeavesATaskTheUserReopened(t *testing.T) {
	f := readChatOn(t, "contact", bookedChat)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
	f.undo(t, f.receipt(t))
	f.readScreenshot(t, bookedChat)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
	if done := f.taskNow(t, f.task).IsDone; done == nil || *done {
		t.Fatal("a second chat closed the task the user had reopened")
	}
}
