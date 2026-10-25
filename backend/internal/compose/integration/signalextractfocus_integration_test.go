// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The second, focused reading of one pass. A promise the first answer left out
// because a later message carried another event is still read. It is read in
// the same pass, before the conversation is marked read.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const recordingPromise = "I will send you the edited recording once it is ready."

type promiseThread struct {
	buyer, promise, need, followup ids.UUID
}

// seedPromiseThread files the conversation of the defect: our promise, the
// contact's reply raising a new need, and our short follow-up.
func seedPromiseThread(t *testing.T, e *Env, company ids.UUID, key string) promiseThread {
	t.Helper()
	buyer := employeeOf(t, e, company, key+" buyer")
	out := promiseThread{buyer: buyer}
	out.promise = captureCommitmentMessage(t, e, buyer, key, "Recording "+key, recordingPromise, "outbound", extractClock.Add(-72*time.Hour))
	party(t, out.promise, "from", &e.Rep1, nil)
	party(t, out.promise, "to", nil, &buyer)
	out.need = captureCommitmentMessage(t, e, buyer, key, "Re: Recording "+key, "We would like to join the beta.", "inbound", extractClock.Add(-60*time.Hour))
	party(t, out.need, "from", nil, &buyer)
	out.followup = captureCommitmentMessage(t, e, buyer, key, "Re: Recording "+key, "I will ask our product team to get in touch.", "outbound", extractClock.Add(-48*time.Hour))
	party(t, out.followup, "from", &e.Rep1, nil)
	party(t, out.followup, "to", nil, &buyer)
	return out
}

func promiseEvent(e *Env, message ids.UUID) map[string]any {
	return map[string]any{
		"kind": "commitment_made", "message_id": message.String(), "promiser_ref": "seat:" + e.Rep1.String(),
		"summary": "Send the edited recording", "quote": recordingPromise, "due_date": "2026-06-05",
		"confidence": 0.95, "commitment": OpenCommitmentMeaning("2026-06-05", recordingPromise),
	}
}

func needEvent(message ids.UUID) map[string]any {
	return map[string]any{
		"kind": "new_opportunity", "message_id": message.String(),
		"summary": "They want to join the beta.", "confidence": 0.95,
	}
}

func eventsReply(t *testing.T, events ...map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatalf("build the scripted reply: %v", err)
	}
	return string(body)
}

func isFocused(req model.Request) bool {
	return strings.Contains(req.Messages[0].Content, focusedReadingMarker)
}

// focusLine is the instruction naming the messages a focused request is for.
func focusLine(t *testing.T, req model.Request) string {
	t.Helper()
	for line := range strings.SplitSeq(req.Messages[0].Content, "\n") {
		if strings.HasPrefix(line, focusedReadingMarker) {
			return line
		}
	}
	t.Fatal("the request carries no focus")
	return ""
}

func openTasks(t *testing.T, e *Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND NOT is_done AND archived_at IS NULL`)
}

func TestAPromiseLeftOutOfTheFirstReadingIsReadOnItsOwn(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	conv := seedPromiseThread(t, e, company, "thread-promise")
	brain := &scriptedBrain{replies: []string{
		eventsReply(t, needEvent(conv.need)),
		eventsReply(t, promiseEvent(e, conv.promise)),
	}}

	extractPass(t, e, brain)

	if brain.calls != 2 {
		t.Fatalf("the model was called %d times, want the first reading and one focused reading", brain.calls)
	}
	if got := openTasks(t, e); got != 1 {
		t.Errorf("the promise left out of the first reading produced %d tasks, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind IN ('new_opportunity','commitment_made')`); got != 2 {
		t.Errorf("%d signals were filed, want the opportunity and the commitment", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal_thread_scan WHERE thread_key='thread-promise'`); got != 1 {
		t.Errorf("the conversation was not marked read after both readings")
	}
	line := focusLine(t, brain.requests[1])
	if !strings.Contains(line, conv.promise.String()) || !strings.Contains(line, conv.followup.String()) || strings.Contains(line, conv.need.String()) {
		t.Errorf("the focus must be the two messages no event cited: %s", line)
	}
	if strings.Contains(brain.requests[1].Messages[0].Content, conv.need.String()) {
		t.Error("the second reading still sends the message whose only finding was the new need")
	}
}

func TestTwoSendsOfOnePromiseEachBecomeATask(t *testing.T) {
	e := Setup(t)
	short := seedPromiseThread(t, e, e.SeedCompany(t, "Acme", &e.Rep1), "thread-short")

	longCompany := e.SeedCompany(t, "Globex", &e.Rep1)
	long := seedPromiseThread(t, e, longCompany, "thread-long")
	// Six more messages push the promise out of the newest window, so only a
	// later pass reads it.
	for i := range 6 {
		m := captureCommitmentMessage(t, e, long.buyer, "thread-long", "Re: Recording thread-long", "Status update.", "inbound",
			extractClock.Add(-47*time.Hour+time.Duration(i)*time.Minute))
		party(t, m, "from", nil, &long.buyer)
	}
	brain := &scriptedBrain{respond: func(req model.Request) string {
		content := req.Messages[0].Content
		switch {
		case strings.Contains(content, short.promise.String()):
			if isFocused(req) {
				return eventsReply(t, promiseEvent(e, short.promise))
			}
			return eventsReply(t, needEvent(short.need))
		case strings.Contains(content, long.promise.String()) && !isFocused(req):
			return eventsReply(t, promiseEvent(e, long.promise))
		case strings.Contains(content, long.need.String()) && !isFocused(req):
			return eventsReply(t, needEvent(long.need))
		}
		return `{"events": []}`
	}}

	for range 4 {
		extractPass(t, e, brain)
	}

	for name, promise := range map[string]ids.UUID{"short": short.promise, "long": long.promise} {
		if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, promise); got != 1 {
			t.Errorf("the %s conversation's promise has %d claims, want 1", name, got)
		}
	}
	if got := openTasks(t, e); got != 2 {
		t.Errorf("two sends of one promise produced %d tasks, want one for each", got)
	}
}

func TestAQuietReadingIsNotReadTwice(t *testing.T) {
	e := Setup(t)
	seedPromiseThread(t, e, e.SeedCompany(t, "Acme", &e.Rep1), "thread-quiet")
	brain := &scriptedBrain{reply: `{"events": []}`}

	extractPass(t, e, brain)

	if brain.calls != 1 {
		t.Errorf("an empty answer cost %d calls, want 1", brain.calls)
	}
}

func TestAMessageAnEarlierPassJudgedIsNotReadAgain(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	conv := seedPromiseThread(t, e, company, "thread-grows")
	brain := &scriptedBrain{replies: []string{
		eventsReply(t, needEvent(conv.need)), `{"events": []}`,
	}}
	extractPass(t, e, brain)

	later := captureCommitmentMessage(t, e, conv.buyer, "thread-grows", "Re: Recording", "Any news?", "inbound", extractClock.Add(-24*time.Hour))
	party(t, later, "from", nil, &conv.buyer)
	brain.replies = []string{eventsReply(t, needEvent(conv.need)), `{"events": []}`}
	extractPass(t, e, brain)

	if brain.calls != 4 {
		t.Fatalf("the model was called %d times, want two readings in each of two passes", brain.calls)
	}
	// The follow-up is the newest message the first pass read. It sits at the
	// watermark instant, which later mail can share, so it is read again.
	line := focusLine(t, brain.requests[3])
	if !strings.Contains(line, later.String()) || !strings.Contains(line, conv.followup.String()) ||
		strings.Contains(line, conv.promise.String()) || strings.Contains(line, conv.need.String()) {
		t.Errorf("the second pass must focus on the new message and the one at the watermark: %s", line)
	}
}

func TestARefusedFocusedReadingKeepsTheThreadDue(t *testing.T) {
	e := Setup(t)
	conv := seedPromiseThread(t, e, e.SeedCompany(t, "Acme", &e.Rep1), "thread-refused")
	brain := &scriptedBrain{replies: []string{
		eventsReply(t, needEvent(conv.need)), eventsReply(t, needEvent(conv.need)),
	}}

	extractPass(t, e, brain)

	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='new_opportunity'`); got != 1 {
		t.Errorf("the first reading's signal was not written: %d", got)
	}
	if got := e.WsScalar(t, `SELECT refusals::text FROM signal_thread_scan WHERE thread_key='thread-refused'`); got != "1" {
		t.Errorf("refusals = %s, want the refused reading counted once", got)
	}
	again := &scriptedBrain{replies: []string{eventsReply(t, needEvent(conv.need)), `{"events": []}`}}
	if pass := extractPassStats(t, e, again); pass.Due != 1 {
		t.Errorf("the thread was offered %d times after a refused reading, want it due again", pass.Due)
	}
	if pass := extractPassStats(t, e, again); pass.Due != 0 {
		t.Errorf("an answered conversation is still due")
	}
}

func TestTwoReadingsTogetherKeepTheEventCap(t *testing.T) {
	e := Setup(t)
	conv := seedPromiseThread(t, e, e.SeedCompany(t, "Acme", &e.Rep1), "thread-cap")
	brain := &scriptedBrain{replies: []string{
		eventsReply(t, needEvent(conv.need), needEvent(conv.followup)),
		eventsReply(t, needEvent(conv.promise), needEvent(conv.promise)),
	}}

	extractPass(t, e, brain)

	if got := e.WsScalar(t, `SELECT refusals::text FROM signal_thread_scan WHERE thread_key='thread-cap'`); got != "1" {
		t.Errorf("refusals = %s, want the over-cap focused reply refused", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='new_opportunity'`); got != 2 {
		t.Errorf("%d signals filed, want only the first reading's two", got)
	}
}

func TestAConversationMadePrivateDuringTheSecondReadingFilesNoSharedSignal(t *testing.T) {
	e := Setup(t)
	conv := seedPromiseThread(t, e, e.SeedCompany(t, "Acme", &e.Rep1), "thread-private")
	brain := &scriptedBrain{replies: []string{
		eventsReply(t, needEvent(conv.need)), eventsReply(t, needEvent(conv.followup)),
	}}
	brain.onCall = func(call int) {
		if call == 2 {
			e.WsExec(t, `UPDATE contact SET visibility='owner', owner_id=$2 WHERE id=$1`, conv.buyer, e.Rep1)
		}
	}

	extractPass(t, e, brain)

	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='new_opportunity' AND visibility='owner' AND owner_id=$1`, e.Rep1); got != 2 {
		t.Errorf("%d signals are private to the member, want both", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE visibility<>'owner'`); got != 0 {
		t.Errorf("%d signals were filed for the whole team", got)
	}
}
