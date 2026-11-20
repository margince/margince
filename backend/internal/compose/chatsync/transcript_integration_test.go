// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
)

func minutesIn(minutes int) time.Time { return linkedAt.Add(time.Duration(minutes) * time.Minute) }

// talkWithAThread seeds a closed stretch of talk in the acme channel. A
// colleague asks; the thread under it holds an outside reply and a reply with
// a file. Then come the chat's own lines, a second topic and a shared file.
func talkWithAThread(e *pullEnv) {
	e.history.Seed("acme",
		dispacttest.Message{
			Seq: 1, UserName: "Rita Rep", UserEmail: "rep@chat.test", ReplyCount: 2, PostedAt: minutesIn(60),
			Text: "MARK-ROOT customer asks to move go-live to April",
		},
		dispacttest.Message{
			Seq: 2, UserName: "Carla", UserEmail: "carla@acme.test", ThreadSeq: 1, PostedAt: minutesIn(61),
			Text: "MARK-REPLY-ONE yes if legal signs",
		},
		dispacttest.Message{
			Seq: 3, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "member_added", PostedAt: minutesIn(61),
			Text: "MARK-JOIN Margince was added by Rita Rep",
		},
		dispacttest.Message{
			Seq: 4, UserName: "Gus", ThreadSeq: 1, Subtype: "thread_broadcast", PostedAt: minutesIn(62),
			Text: "MARK-REPLY-BCAST pdf attached", Files: []dispacttest.File{{Name: "MARKfile-quote2.pdf", MimeType: "application/pdf"}},
		},
		dispacttest.Message{
			Seq: 5, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "channel_name", PostedAt: minutesIn(62),
			Text: "MARK-RENAME renamed the channel",
		},
		dispacttest.Message{
			Seq: 6, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "member_removed", PostedAt: minutesIn(63),
			Text: "MARK-LEAVE removed Sam",
		},
		dispacttest.Message{
			Seq: 7, UserName: "Tia", UserEmail: "tia@acme.test", PostedAt: minutesIn(63),
			Text: "MARK-TOPLEVEL separate topic about invoices",
		},
		dispacttest.Message{
			Seq: 8, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "file_share", PostedAt: minutesIn(64),
			Text: "MARK-SHARE the signed order", Files: []dispacttest.File{{Name: "MARKfile-order.pdf", MimeType: "application/pdf"}},
		},
	)
}

// pulledTalk pulls the talk and returns the one request the episode sent.
func pulledTalk(t *testing.T, e *pullEnv) string {
	t.Helper()
	rec := &recordingSummary{}
	e.puller.Lane = rec
	e.pull(t)
	if len(rec.prompts) != 1 {
		t.Fatalf("model requests = %d, want one for the closed episode", len(rec.prompts))
	}
	return rec.prompts[0]
}

// requireMarked fails unless the line holding marker opens with the given
// message attribute, such as `message="#2 outside, replying to #1"`.
func requireMarked(t *testing.T, prompt, marker, attribute string) {
	t.Helper()
	if line := lineWith(prompt, marker); !strings.Contains(line, " "+attribute+">") {
		t.Errorf("line %q, want it opened with %s", line, attribute)
	}
}

func TestThePulledTalkPlacesEachAuthorAndKeepsTheirEmailOut(t *testing.T) {
	e := setupPull(t)
	e.exec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('gradion.test', 'admin', true)`)
	e.exec(t, `INSERT INTO app_user (id, email, display_name, status) VALUES (uuidv7(), 'dee@gradion.test', 'Dee Departed', 'deactivated')`)
	// A seat may type a customer's address as its own alias; that proves nothing.
	e.exec(t, `INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, 'address', 'tia@acme.test', 'user', 'human:rep')`, e.rep)
	authors := []struct{ name, email, side string }{
		{"Rita Rep", "rep@chat.test", "colleague"},
		{"Rita Again", "REP@Chat.Test", "colleague"},
		{"Nora", "nora@gradion.test", "colleague"},
		{"Nora Loud", "NORA@GRADION.TEST", "colleague"},
		{"Dee Departed", "dee@gradion.test", "colleague"},
		{"Sub", "sub@eu.gradion.test", "unknown"},
		{"Carla", "carla@acme.test", "outside"},
		{"Tia", "tia@acme.test", "outside"},
		{"Gus", "", "unknown"},
		{"", "ghost@acme.test", "outside"},
	}
	for i, a := range authors {
		e.history.Seed("acme", dispacttest.Message{
			Seq: int64(i + 1), UserName: a.name, UserEmail: a.email, PostedAt: minutesIn(60 + i), Text: "MARK-AUTHOR-" + strconv.Itoa(i+1) + ".",
		})
	}

	prompt := pulledTalk(t, e)

	for i, a := range authors {
		n := strconv.Itoa(i + 1)
		requireMarked(t, prompt, "MARK-AUTHOR-"+n+".", `message="#`+n+" "+a.side+`"`)
	}
	if line := lineWith(prompt, "MARK-AUTHOR-10."); !strings.Contains(line, ">someone: ") {
		t.Errorf("line %q, want a nameless author shown as someone", line)
	}
	lowered := strings.ToLower(prompt)
	for _, a := range authors {
		if a.email != "" && strings.Contains(lowered, strings.ToLower(a.email)) {
			t.Errorf("the request carries the email %q", a.email)
		}
	}
}

func TestThePulledTalkKeepsTheThreadAndNamesTheFiles(t *testing.T) {
	e := setupPull(t)
	talkWithAThread(e)

	prompt := pulledTalk(t, e)

	requireMarked(t, prompt, "MARK-ROOT", `message="#1 colleague"`)
	requireMarked(t, prompt, "MARK-REPLY-ONE", `message="#2 outside, replying to #1"`)
	requireMarked(t, prompt, "MARK-REPLY-BCAST", `message="#3 unknown, replying to #1"`)
	requireMarked(t, prompt, "MARK-TOPLEVEL", `message="#4 outside"`)
	requireMarked(t, prompt, "MARK-SHARE", `message="#5 colleague"`)
	for file, message := range map[string]string{"MARKfile-quote2.pdf": "#3", "MARKfile-order.pdf": "#5"} {
		line := lineWith(prompt, file)
		if !strings.HasPrefix(line, "file <") || !strings.Contains(line, ` file_of="`+message+`">`) || !strings.Contains(line, "application/pdf") {
			t.Errorf("file line %q, want it under %s with its type", line, message)
		}
	}
}

func TestTheChatsOwnLinesNeverReachTheModel(t *testing.T) {
	e := setupPull(t)
	talkWithAThread(e)

	prompt := pulledTalk(t, e)

	requireNotIn(t, prompt, "MARK-JOIN", "MARK-RENAME", "MARK-LEAVE")
}

// A member filing one thread, or one shared file, gets the same transcript as
// a pull.
func TestAFiledThreadAndAFiledFileShareAreSentAsTheyWereWritten(t *testing.T) {
	e := setupPull(t)
	rec := &recordingSummary{}
	e.filer.Lane = rec
	talkWithAThread(e)

	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", true); err != nil {
		t.Fatalf("filing the thread: %v", err)
	}
	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "8", false); err != nil {
		t.Fatalf("filing the shared file: %v", err)
	}

	if len(rec.prompts) != 2 {
		t.Fatalf("model requests = %d, want two", len(rec.prompts))
	}
	thread, share := rec.prompts[0], rec.prompts[1]
	requireMarked(t, thread, "MARK-REPLY-ONE", `message="#2 outside, replying to #1"`)
	requireIn(t, thread, "MARKfile-quote2.pdf")
	requireNotIn(t, thread, "MARK-JOIN", "MARK-TOPLEVEL", "carla@acme.test")
	requireMarked(t, share, "MARK-SHARE", `message="#1 colleague"`)
	requireIn(t, share, "MARKfile-order.pdf")
}

// lineWith is the prompt line that holds marker, or the empty string.
func lineWith(prompt, marker string) string {
	for line := range strings.SplitSeq(prompt, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	return ""
}
