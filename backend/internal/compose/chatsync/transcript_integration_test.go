// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
)

// talkWithAThread seeds a closed stretch of talk in the acme channel. A
// colleague asks; the thread under it holds an outside reply and a reply with
// a file. Then come the chat's own lines and a second topic.
func talkWithAThread(e *pullEnv) {
	at := func(minutes int) time.Time { return linkedAt.Add(time.Duration(minutes) * time.Minute) }
	e.history.Seed("acme",
		dispacttest.Message{
			Seq: 1, UserName: "Rita Rep", UserEmail: "rep@chat.test", ReplyCount: 2, PostedAt: at(60),
			Text: "MARK-ROOT customer asks to move go-live to April",
		},
		dispacttest.Message{
			Seq: 2, UserName: "Carla", UserEmail: "carla@acme.test", ThreadSeq: 1, PostedAt: at(61),
			Text: "MARK-REPLY-ONE yes if legal signs",
		},
		dispacttest.Message{
			Seq: 3, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "member_added", PostedAt: at(61),
			Text: "MARK-JOIN Margince was added by Rita Rep",
		},
		dispacttest.Message{
			Seq: 4, UserName: "Gus", ThreadSeq: 1, Subtype: subtypeBroadcast, PostedAt: at(62),
			Text: "MARK-REPLY-BCAST pdf attached", Files: []dispacttest.File{{Name: "MARKfile-quote2.pdf", MimeType: "application/pdf"}},
		},
		dispacttest.Message{
			Seq: 5, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "channel_name", PostedAt: at(62),
			Text: "MARK-RENAME renamed the channel",
		},
		dispacttest.Message{
			Seq: 6, UserName: "Rita Rep", UserEmail: "rep@chat.test", Subtype: "member_removed", PostedAt: at(63),
			Text: "MARK-LEAVE removed Sam",
		},
		dispacttest.Message{
			Seq: 7, UserName: "Tia", UserEmail: "tia@acme.test", PostedAt: at(63),
			Text: "MARK-TOPLEVEL separate topic about invoices",
		},
	)
}

// pulledTalk pulls the talk and answers the one request the episode sent.
func pulledTalk(t *testing.T, e *pullEnv, rec *recordingSummary) string {
	t.Helper()
	e.pull(t)
	if len(rec.prompts) != 1 {
		t.Fatalf("model requests = %d, want one for the closed episode", len(rec.prompts))
	}
	return rec.prompts[0]
}

func TestThePulledTalkMarksEachAuthorsSideAndKeepsTheirEmailOut(t *testing.T) {
	e := setupPull(t)
	rec := &recordingSummary{}
	e.puller.Lane = rec
	// A seat may type a customer's address as its own alias; that proves nothing.
	e.exec(t, `INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, 'address', 'tia@acme.test', 'user', 'human:rep')`, e.rep)
	talkWithAThread(e)

	prompt := pulledTalk(t, e, rec)

	for marker, side := range map[string]string{
		"MARK-ROOT": "colleague", "MARK-REPLY-ONE": "outside", "MARK-REPLY-BCAST": "unknown", "MARK-TOPLEVEL": "outside",
	} {
		if line := lineWith(prompt, marker); !strings.Contains(line, " "+side+" <") {
			t.Errorf("line %q, want its author marked %s", line, side)
		}
	}
	requireNotIn(t, prompt, "rep@chat.test", "carla@acme.test", "tia@acme.test")
}

func TestThePulledTalkKeepsTheThreadAndNamesTheFile(t *testing.T) {
	e := setupPull(t)
	rec := &recordingSummary{}
	e.puller.Lane = rec
	talkWithAThread(e)

	prompt := pulledTalk(t, e, rec)

	root := lineWith(prompt, "MARK-ROOT")
	if !strings.HasPrefix(root, "#1 ") || strings.Contains(root, "replying") {
		t.Errorf("root line %q, want #1 and no reply", root)
	}
	for _, reply := range []string{"MARK-REPLY-ONE", "MARK-REPLY-BCAST"} {
		if line := lineWith(prompt, reply); !strings.Contains(line, "replying to #1") {
			t.Errorf("line %q, want it to answer #1", line)
		}
	}
	if line := lineWith(prompt, "MARK-TOPLEVEL"); strings.Contains(line, "replying") {
		t.Errorf("line %q, want a message of its own", line)
	}
	file := lineWith(prompt, "MARKfile-quote2.pdf")
	if !strings.HasPrefix(file, "#3 file: ") || !strings.Contains(file, "application/pdf") {
		t.Errorf("file line %q, want the PDF under the reply #3 with its type", file)
	}
}

func TestTheChatsOwnLinesNeverReachTheModel(t *testing.T) {
	e := setupPull(t)
	rec := &recordingSummary{}
	e.puller.Lane = rec
	talkWithAThread(e)

	prompt := pulledTalk(t, e, rec)

	requireNotIn(t, prompt, "MARK-JOIN", "MARK-RENAME", "MARK-LEAVE")
	if line := lineWith(prompt, "MARK-TOPLEVEL"); !strings.HasPrefix(line, "#4 ") {
		t.Errorf("line %q, want #4: the three chat lines take no number", line)
	}
}

// A member filing one thread gets the same transcript as a pull.
func TestAFiledThreadIsSentWithItsSidesRepliesAndFile(t *testing.T) {
	e := setupPull(t)
	rec := &recordingSummary{}
	e.filer.Lane = rec
	talkWithAThread(e)

	if _, err := e.filer.FileMessage(e.memberCtx(), chatlinks.RecordDeal, e.deal, "1", true); err != nil {
		t.Fatalf("filing the thread: %v", err)
	}

	if len(rec.prompts) != 1 {
		t.Fatalf("model requests = %d, want one", len(rec.prompts))
	}
	prompt := rec.prompts[0]
	requireIn(t, prompt, "replying to #1", "MARKfile-quote2.pdf")
	requireNotIn(t, prompt, "MARK-JOIN", "MARK-TOPLEVEL", "carla@acme.test")
	if line := lineWith(prompt, "MARK-REPLY-ONE"); !strings.Contains(line, " outside <") {
		t.Errorf("line %q, want the outside author marked", line)
	}
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
