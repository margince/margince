// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The reply verdict, its backlog and its two write doors, against a real
// database.
//
// Nothing in the unit lane reaches any of it. The inbound restriction is a CHECK
// constraint the database evaluates, the CAS is a WHERE clause, and the history
// row is written in the same transaction as the column — each can be wrong in a
// way that compiles and simply judges the wrong messages, or records a rate
// nobody can later explain.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A verdict belongs to a message somebody sent US.
//
// Judging our own outbound mail would put the workspace's own words into a rate
// about what customers said, and the error flatters: an SDR's cheerful follow-up
// reads positive. The constraint is in the database rather than in the writer's
// memory, so this asserts the refusal rather than the writer's good intentions.
func TestAnOutboundMessageCannotCarryAReplyVerdict(t *testing.T) {
	e := setupLoad(t)
	outbound := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Following up', now() - interval '1 day', $2, 'seed', 'system')`,
		outbound, "thread-"+outbound.String())

	applied, err := storeKnowing(e).SetReplyVerdict(asReplyClassifier(e), outbound, ReplyVerdictPositive, "test-model")
	// The write door's own WHERE excludes it, so this is a clean no-op rather
	// than a constraint violation: applied=false, no error, nothing written.
	if err != nil {
		t.Fatalf("setting a verdict on outbound mail should be refused quietly, got: %v", err)
	}
	if applied {
		t.Error("an outbound message took a reply verdict — we wrote it, so it answers nobody")
	}
	if v := replyVerdictOf(t, e, outbound); v != nil {
		t.Errorf("outbound message carries verdict %q", *v)
	}
}

// The verdict lands, and it lands WITH the classifier that reached it.
//
// The version is the half that is easy to drop and expensive to miss: a
// positive-reply rate that moved after a model change and one that moved because
// customers changed their minds are different facts, and without this column
// nobody can tell them apart afterwards.
func TestAnInboundReplyTakesAVerdictAndNamesItsJudge(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	reply := e.waitingFrom(t, "Yes, let us talk next week", "buyer@customer.test", person)

	applied, err := storeKnowing(e).SetReplyVerdict(asReplyClassifier(e), reply, ReplyVerdictPositive, "local-small-v3")
	if err != nil {
		t.Fatalf("setting the reply verdict: %v", err)
	}
	if !applied {
		t.Fatal("an inbound reply did not take a verdict")
	}

	got := replyVerdictOf(t, e, reply)
	if got == nil || *got != ReplyVerdictPositive {
		t.Errorf("verdict = %v, want %q", got, ReplyVerdictPositive)
	}
	if judge := replyJudgeOf(t, e, reply); judge != "local-small-v3" {
		t.Errorf("judge = %q, want the classifier that reached it", judge)
	}

	// The history carries the classifier's own entry from the start, so the
	// first human correction has something to disagree WITH.
	entries := replyHistoryOf(t, e, reply)
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1 (the classifier's own): %+v", len(entries), entries)
	}
	if entries[0].isHuman {
		t.Error("the classifier's own entry is recorded as a human correction")
	}
}

// A second pass does NOT overwrite a standing verdict.
//
// The classifier reads a batch, spends a model call and writes back; two passes
// can overlap on one row. The earlier verdict stands, because a rate that
// silently re-judged its own history would move without anybody changing
// anything.
func TestASecondReplyVerdictDoesNotOverwriteTheFirst(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	reply := e.waitingFrom(t, "Not for us, thanks", "buyer@customer.test", person)
	s := storeKnowing(e)

	if _, err := s.SetReplyVerdict(asReplyClassifier(e), reply, ReplyVerdictNegative, "first"); err != nil {
		t.Fatalf("first verdict: %v", err)
	}
	applied, err := s.SetReplyVerdict(asReplyClassifier(e), reply, ReplyVerdictPositive, "second")
	if err != nil {
		t.Fatalf("second verdict: %v", err)
	}

	if applied {
		t.Error("a second pass overwrote a standing verdict")
	}
	if got := replyVerdictOf(t, e, reply); got == nil || *got != ReplyVerdictNegative {
		t.Errorf("verdict = %v, want the first one (%q) to stand", got, ReplyVerdictNegative)
	}
}

// A human correction APPENDS. The activity carries the new answer so every read
// stays one row; the history says how it got there.
//
// This is the invariant the whole history table exists for. A rate that moved
// because a rep re-judged twenty replies is a different fact from one that moved
// because customers did, and an overwriting column cannot tell them apart.
func TestAHumanCorrectionAppendsRatherThanReplaces(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	reply := e.waitingFrom(t, "Interesting, but not now", "buyer@customer.test", person)
	s := storeKnowing(e)

	if _, err := s.SetReplyVerdict(asReplyClassifier(e), reply, ReplyVerdictPositive, "local-small-v3"); err != nil {
		t.Fatalf("the classifier's verdict: %v", err)
	}
	corrected := ReplyVerdictNeutral
	if err := s.CorrectReplyVerdict(asHumanCorrector(e), reply, &corrected); err != nil {
		t.Fatalf("correcting the verdict: %v", err)
	}

	// The row now reads as the human left it.
	if got := replyVerdictOf(t, e, reply); got == nil || *got != ReplyVerdictNeutral {
		t.Errorf("verdict = %v, want the correction (%q)", got, ReplyVerdictNeutral)
	}

	// And BOTH judgements survive, in order. Asserting only the current value
	// would pass just as well against an overwriting column, which is the shape
	// this table exists to refuse.
	entries := replyHistoryOf(t, e, reply)
	if len(entries) != 2 {
		t.Fatalf("history entries = %d, want 2 (classifier then human): %+v", len(entries), entries)
	}
	if entries[0].isHuman || entries[0].verdict == nil || *entries[0].verdict != ReplyVerdictPositive {
		t.Errorf("first entry = %+v, want the classifier's positive", entries[0])
	}
	if !entries[1].isHuman || entries[1].verdict == nil || *entries[1].verdict != ReplyVerdictNeutral {
		t.Errorf("second entry = %+v, want the human's neutral", entries[1])
	}
}

// A human may correct a reply back to UNJUDGED. "Nobody can tell from this
// message" is an answer, and it returns the row to counting in neither half of
// the rate rather than leaving a guess standing.
func TestAHumanMayCorrectAVerdictBackToUnjudged(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	reply := e.waitingFrom(t, "?", "buyer@customer.test", person)
	s := storeKnowing(e)

	if _, err := s.SetReplyVerdict(asReplyClassifier(e), reply, ReplyVerdictPositive, "local-small-v3"); err != nil {
		t.Fatalf("the classifier's verdict: %v", err)
	}
	if err := s.CorrectReplyVerdict(asHumanCorrector(e), reply, nil); err != nil {
		t.Fatalf("correcting to unjudged: %v", err)
	}

	if got := replyVerdictOf(t, e, reply); got != nil {
		t.Errorf("verdict = %q, want NULL — unjudged is a real answer", *got)
	}
	// The stamp columns go with it: a moment or a judge left behind would claim
	// a judgement that no longer stands.
	if judge := replyJudgeOf(t, e, reply); judge != "" {
		t.Errorf("judge = %q, want empty once the verdict is withdrawn", judge)
	}
	// The withdrawal is still recorded — the history is how it got here.
	if entries := replyHistoryOf(t, e, reply); len(entries) != 2 {
		t.Errorf("history entries = %d, want 2 (the verdict and its withdrawal)", len(entries))
	}
}

// asReplyClassifier is the context the classify pass runs under: a system
// principal, the way every background job in compose builds one.
func asReplyClassifier(e *loadEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:capture_classify",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// asHumanCorrector is a person disagreeing with the classifier — the correction
// path's real caller, and the reason the history records WHO decided.
func asHumanCorrector(e *loadEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func replyVerdictOf(t *testing.T, e *loadEnv, id ids.UUID) *string {
	t.Helper()
	var out *string
	if err := e.owner.QueryRow(t.Context(),
		`SELECT reply_verdict FROM activity WHERE id = $1`, id).Scan(&out); err != nil {
		t.Fatalf("reading the reply verdict: %v", err)
	}
	return out
}

func replyJudgeOf(t *testing.T, e *loadEnv, id ids.UUID) string {
	t.Helper()
	var out *string
	if err := e.owner.QueryRow(t.Context(),
		`SELECT reply_verdict_by FROM activity WHERE id = $1`, id).Scan(&out); err != nil {
		t.Fatalf("reading the reply verdict's judge: %v", err)
	}
	if out == nil {
		return ""
	}
	return *out
}

type replyHistoryEntry struct {
	verdict *string
	isHuman bool
}

func replyHistoryOf(t *testing.T, e *loadEnv, id ids.UUID) []replyHistoryEntry {
	t.Helper()
	rows, err := e.owner.Query(t.Context(),
		`SELECT verdict, is_human
		   FROM activity_reply_verdict_history
		  WHERE activity_id = $1 ORDER BY decided_at, id`, id)
	if err != nil {
		t.Fatalf("reading the verdict history: %v", err)
	}
	defer rows.Close()
	var out []replyHistoryEntry
	for rows.Next() {
		var entry replyHistoryEntry
		if err := rows.Scan(&entry.verdict, &entry.isHuman); err != nil {
			t.Fatalf("scanning a history entry: %v", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the verdict history: %v", err)
	}
	return out
}
