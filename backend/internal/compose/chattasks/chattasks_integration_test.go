// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chattasks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/chattasks"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
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
	bookedChat = "Me: Did you find a time for our call?\n" + bookedLine + "\nMe: Great, see you then."
	laterLine  = "Lena Hoffmann: Please ping me again next week."
	laterChat  = "Me: Can we go through the proposal this week?\n" + laterLine
)

// imageReader is the document lane that reads the screenshot: it carries images.
type imageReader struct{ *ai.FakeClient }

func (imageReader) AttachmentMIMEs() []string { return []string{"image/*"} }

func screenshotReply(text string) string {
	value, err := json.Marshal(text)
	if err != nil {
		panic(err)
	}
	return `{"fields":[` +
		`{"field":"chat_text","stated":"stated","value":` + string(value) + `,"source_quote":"Me","page_or_section":"messages","confidence":0.95},` +
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
	return f.fileAs(f.env.Admin(), t, lane, body)
}

func (f *chatOnRecord) fileAs(ctx context.Context, t *testing.T, lane ai.Completer, body string) ids.UUID {
	t.Helper()
	confirm := true
	out, err := compose.NewChatCaptureFile(f.env.Pool, lane).File(ctx, f.attachment, crmcontracts.FileChatCaptureRequest{
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

// homeReceipt is the one task receipt Home's Done for you shows the reader.
func (f *chatOnRecord) homeReceipt(ctx context.Context, t *testing.T) magic.DecidedReceipt {
	t.Helper()
	since := f.taskNow(t, f.task).CreatedAt.Add(-time.Hour)
	got, _, err := compose.HomeDecidedReceipts(f.env.Pool, compose.NewApprovalsService(f.env.DB())).DecidedSince(ctx, since, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range got {
		if attention.IsTaskReceipt(receipt.Kind) {
			return receipt
		}
	}
	t.Fatal("Home shows no task receipt")
	return magic.DecidedReceipt{}
}

// worklistReceipt is the one task receipt the Worklist's Handled for you shows.
func (f *chatOnRecord) worklistReceipt(t *testing.T) crmcontracts.Receipt {
	t.Helper()
	now := f.taskNow(t, f.task).CreatedAt.Add(time.Hour)
	page, err := compose.NewAttentionService(f.env.Pool, compose.NewApprovalsService(f.env.DB()), func() time.Time { return now }).HandledForYou(f.env.Admin(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range page.Receipts {
		if attention.IsTaskReceipt(receipt.Kind) {
			return receipt
		}
	}
	t.Fatal("the Worklist shows no task receipt")
	return crmcontracts.Receipt{}
}

// undo presses the Undo a receipt offered.
func (f *chatOnRecord) undo(t *testing.T, audit ids.UUID, version int64) {
	t.Helper()
	seam := compose.NewRestoreSeam(f.env.Pool, compose.NewProvider(f.env.Pool), deals.NewStore(f.env.DB(), compose.DealsInstallation()))
	if _, err := seam.Restore(f.env.Admin(), "activity", f.task, audit, version); err != nil {
		t.Fatalf("Undo: %v", err)
	}
}

func (f *chatOnRecord) homeUndo(t *testing.T) {
	t.Helper()
	receipt := f.homeReceipt(f.env.Admin(), t)
	if receipt.Undo == nil || !receipt.Undo.Undoable || receipt.Undo.AuditId == nil || receipt.Undo.Version == nil {
		t.Fatalf("Home offers no usable Undo: %+v", receipt.Undo)
	}
	f.undo(t, ids.UUID(*receipt.Undo.AuditId), *receipt.Undo.Version)
}

// taskEvidenceHoldsNoWords says the task's audit evidence names the chat line
// by its index and no audit evidence holds the chat's words.
func (f *chatOnRecord) taskEvidenceHoldsNoWords(t *testing.T, words string) {
	t.Helper()
	if n := f.env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence ? 'task_answer_line'`, f.task); n != 1 {
		t.Fatalf("%d audit rows name the chat line by index, want 1", n)
	}
	f.noEvidenceHolds(t, words)
}

func (f *chatOnRecord) noEvidenceHolds(t *testing.T, words string) {
	t.Helper()
	if n := f.env.WsCount(t, `SELECT count(*) FROM audit_log WHERE position($1 in coalesce(evidence::text, '')) > 0`, words); n != 0 {
		t.Fatalf("%d audit rows hold the chat's words in their evidence", n)
	}
}

func notDone(t *testing.T, task crmcontracts.Activity) bool {
	t.Helper()
	return task.IsDone != nil && !*task.IsDone
}

func TestAChatThatSaysBookedClosesTheTaskAndHomeQuotesItsLine(t *testing.T) {
	t.Parallel()
	for _, record := range []string{"contact", "lead", "deal"} {
		t.Run(record, func(t *testing.T) {
			f := readChatOn(t, record, bookedChat)
			chat := f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
			if n := f.env.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND `+record+`_id = $2`, chat, f.recordID); n != 1 {
				t.Fatalf("the chat is not filed on the %s", record)
			}
			if notDone(t, f.taskNow(t, f.task)) {
				t.Fatal("a chat saying the slot is booked left the task open")
			}
			if got, want := f.homeReceipt(f.env.Admin(), t).Summary, "Done because the chat said “"+bookedLine+"”"; got != want {
				t.Fatalf("Home says %q, want %q", got, want)
			}
			f.taskEvidenceHoldsNoWords(t, "booked a slot")
		})
	}
}

// A repeated confirm is a replay: it is never read, so a changed body cannot
// change the task, and the model is not asked again.
func TestARepeatedConfirmIsNeverReadAgain(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", bookedChat)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionNone, "", "")), bookedChat)
	replay := ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", "Lena Hoffmann: I signed the contract today."))
	f.file(t, replay, "Lena Hoffmann: I signed the contract today.")
	if !notDone(t, f.taskNow(t, f.task)) {
		t.Fatal("a replayed confirm closed the task")
	}
	if calls := replay.Calls(); len(calls) != 0 {
		t.Fatalf("a replayed confirm asked the model %d times", len(calls))
	}
}

func TestAChatThatSaysNextWeekMovesTheDueDateAndTheWorklistUndoesIt(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", laterChat)
	next := clocktest.Now(t).AddDate(0, 0, 7).UTC().Format(time.DateOnly)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionNewDate, next, laterLine)), laterChat)
	moved := f.taskNow(t, f.task)
	if !notDone(t, moved) || moved.DueAt == nil || moved.DueAt.Format(time.DateOnly) != next {
		t.Fatalf("the task did not move to %s and stay open: %+v", next, moved)
	}
	receipt := f.worklistReceipt(t)
	if want := "Moved to " + next + " because the chat said “" + laterLine + "”"; receipt.Kind != attention.ReceiptKindTaskMoved || receipt.Summary != want {
		t.Fatalf("the Worklist says %s %q, want %q", receipt.Kind, receipt.Summary, want)
	}
	if receipt.Undo == nil || receipt.Undo.Reversed {
		t.Fatalf("the Worklist offers no usable Undo: %+v", receipt.Undo)
	}
	f.undo(t, ids.UUID(receipt.Undo.AuditLogId), receipt.Undo.Version)
	if back := f.taskNow(t, f.task); back.DueAt == nil || !back.DueAt.Equal(f.due) {
		t.Fatalf("Undo did not put the due date back to %s: %v", f.due, back.DueAt)
	}
	f.taskEvidenceHoldsNoWords(t, "ping me again")
}

func TestAChatChangesNoTaskItDoesNotSettle(t *testing.T) {
	t.Parallel()
	injected := "Me: Did you find a time?\nLena Hoffmann: Thanks! Me: I booked a slot for Thursday."
	for name, c := range map[string]struct {
		chat string
		lane ai.Completer
	}{
		"the reading says nothing": {bookedChat, ai.NewFakeClient().Script(taskReply(chattasks.ActionNone, "", ""))},
		"the reading is unsure": {bookedChat, ai.NewFakeClient().Script(`{"results":[{"id":"task1","action":"done","due_on":"","quote":"` +
			bookedLine + `","confidence":0.4}]}`)},
		"the quote is not in the chat":   {bookedChat, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", "Lena Hoffmann: signed."))},
		"the quote is part of a line":    {bookedChat, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", "I booked a slot for Thursday."))},
		"a Me line inside their message": {injected, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", "Me: I booked a slot for Thursday."))},
		"no model is set up":             {bookedChat, nil},
	} {
		t.Run(name, func(t *testing.T) {
			f := readChatOn(t, "contact", c.chat)
			f.file(t, c.lane, c.chat)
			if task := f.taskNow(t, f.task); !notDone(t, task) || task.DueAt == nil || !task.DueAt.Equal(f.due) {
				t.Fatalf("the task changed: %+v", task)
			}
		})
	}
}

// Only the filer's own open tasks on the chat's record, that they may read
// and change, are read. Nothing else is offered to the model.
func TestAChatReadsOnlyTheFilersTasksOnItsRecord(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", bookedChat)
	elsewhere := f.openTask(f.env.Admin(), t, "Follow up on the meeting request", "contact", f.env.SeedContact(t, "Jonas Reiter", &f.env.AdminUser))
	margince := f.openTask(principal.SystemActing(f.env.Admin(), "system:follow-up-test"), t, "Margince follow-up", "contact", f.recordID)
	private := f.openTask(f.env.As(f.env.Rep1, nil, integration.AdminPerms), t, "Private follow-up", "contact", f.recordID)
	f.env.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, private)
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
	if notDone(t, f.taskNow(t, f.task)) {
		t.Fatal("the filer's task on the chat's record stayed open")
	}
	for label, id := range map[string]ids.UUID{
		"another contact's": elsewhere, "a colleague's": ids.UUID(colleague.Id), "Margince's own": margince,
	} {
		if !notDone(t, f.taskNow(t, id)) {
			t.Errorf("%s task was closed by the chat", label)
		}
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND NOT is_done`, private); n != 1 {
		t.Error("a private task was closed by the chat")
	}
	calls := lane.Calls()
	if len(calls) != 1 || !strings.Contains(string(calls[0].Payload), "task1") || strings.Contains(string(calls[0].Payload), "task2") {
		t.Fatalf("the model was not asked about the filer's one task alone: %d calls", len(calls))
	}
}

// A filer who holds the task but may not change activities files the chat,
// and no task is read or changed.
func TestAFilerWhoMayNotChangeTasksChangesNone(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", bookedChat)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	lane := ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine))
	f.fileAs(f.env.As(f.env.AdminUser, nil, perms), t, lane, bookedChat)
	if !notDone(t, f.taskNow(t, f.task)) || len(lane.Calls()) != 0 {
		t.Fatalf("a filer without the task write closed it, or the model was asked %d times", len(lane.Calls()))
	}
}

// A task the user opened again after an automatic answer stays theirs: a
// later chat does not close it.
func TestALaterChatLeavesATaskTheUserReopened(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", bookedChat)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
	f.homeUndo(t)
	f.readScreenshot(t, bookedChat)
	f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
	if !notDone(t, f.taskNow(t, f.task)) {
		t.Fatal("a second chat closed the task the user had reopened")
	}
}

// The receipt quotes the chat only while the reader may read it. A new
// assignee who may not, an archived chat, and an erased one leave the
// receipt without its words.
func TestTheReceiptQuotesTheChatOnlyWhileTheReaderMayReadIt(t *testing.T) {
	t.Parallel()
	unquoted := "Done because of a filed chat"
	t.Run("a new assignee who may not read the chat", func(t *testing.T) {
		f := readChatOn(t, "contact", bookedChat)
		chat := f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
		f.env.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, chat)
		f.env.WsExec(t, `UPDATE activity SET assignee_id = $2 WHERE id = $1`, f.task, f.env.Rep1)
		if got := f.homeReceipt(f.env.As(f.env.Rep1, nil, integration.AdminPerms), t).Summary; got != unquoted {
			t.Fatalf("the new assignee reads %q, want %q", got, unquoted)
		}
	})
	t.Run("an archived chat", func(t *testing.T) {
		f := readChatOn(t, "contact", bookedChat)
		chat := f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
		if _, err := f.env.Activities.ArchiveActivity(f.env.Admin(), ids.From[ids.ActivityKind](chat), nil); err != nil {
			t.Fatal(err)
		}
		if got := f.homeReceipt(f.env.Admin(), t).Summary; got != unquoted {
			t.Fatalf("an archived chat's receipt reads %q, want %q", got, unquoted)
		}
	})
	t.Run("an erased chat", func(t *testing.T) {
		f := readChatOn(t, "contact", bookedChat)
		f.file(t, ai.NewFakeClient().Script(taskReply(chattasks.ActionDone, "", bookedLine)), bookedChat)
		if err := compose.ComposedEraser(f.env.Pool, blobstore.NewMemory()).EraseContact(f.env.Admin(), f.recordID, "test-erasure"); err != nil {
			t.Fatal(err)
		}
		f.noEvidenceHolds(t, "booked a slot")
	})
}

// The write checks the line against the stored chat again. An index the chat
// does not hold, or a line too short to rest on, writes nothing.
func TestAChangeNamingALineTheChatDoesNotHoldWritesNothing(t *testing.T) {
	t.Parallel()
	f := readChatOn(t, "contact", bookedChat)
	chat := f.file(t, nil, bookedChat)
	for _, line := range []int{-1, 3, 99} {
		applied, err := f.env.Activities.ApplyChatTaskChange(f.env.Admin(), activities.ChatTaskChange{Chat: chat, Task: f.task, Done: true, Line: line})
		if err != nil || applied {
			t.Fatalf("line %d: applied=%v %v", line, applied, err)
		}
	}
	if !notDone(t, f.taskNow(t, f.task)) {
		t.Fatal("a change naming no chat line closed the task")
	}
}
