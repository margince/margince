// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const geminiNotes = "gemini-notes@google.com"

// notesMail captures one Gemini notes mail to the seat through the production
// sink. authenticated adds Gmail's DMARC pass for google.com; without it the
// From is only what the sender typed.
func (o *owedEnv) notesMail(t *testing.T, authenticated bool) ids.UUID {
	t.Helper()
	return o.capture(t, owedMail{
		from: geminiNotes, to: o.seat, subject: "Notes: Quarterly review", at: o.now,
		messageID: "notes-" + ids.NewV7().String() + "@google.com", authenticated: authenticated,
		body: "Summary. Pat agreed to send the signed order by Friday.",
	})
}

// judgeAsNoise runs a verdict pass whose model calls the pending notes sender
// transactional, then the sweep that hides mail from noise senders.
func (o *owedEnv) judgeAsNoise(t *testing.T) *scriptedVerdictBrain {
	t.Helper()
	return o.judgeAs(t, capture.KindTransactional)
}

func (o *owedEnv) judgeAs(t *testing.T, kind string) *scriptedVerdictBrain {
	t.Helper()
	pending := o.e.WsScalar(t, `
		SELECT coalesce(max(id::text), '') FROM capture_pending_counterparty
		 WHERE email = $1 AND status = 'pending'`, geminiNotes)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{pending: kind}}
	engine := NewCounterpartyVerdictEngine(o.e.Pool, brain, CaptureConfig{}, slog.Default())
	ctx := principal.WithWorkspaceID(context.Background(), o.e.WS)
	if err := engine.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if err := engine.HideNoiseStragglersWorkspace(ctx); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}
	return brain
}

func (o *owedEnv) visible(t *testing.T, id ids.UUID) bool {
	t.Helper()
	return countIn(t, o.e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, id) == 1
}

func TestAuthenticatedGoogleNotesMailIsNeverJudgedNoise(t *testing.T) {
	o := setupOwed(t)
	notes := o.notesMail(t, true)

	brain := o.judgeAsNoise(t)

	if !o.visible(t, notes) {
		t.Fatal("a model's noise answer hid Google's authenticated meeting notes")
	}
	if brain.calls != 0 {
		t.Fatalf("the model was asked %d times about a sender a fixed rule settles", brain.calls)
	}
	o.settledAsRoleMailbox(t)
}

func (o *owedEnv) settledAsRoleMailbox(t *testing.T) {
	t.Helper()
	if n := countIn(t, o.e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE email = $1 AND status = 'real' AND kind = $2`, geminiNotes, capture.KindRoleMailbox); n != 1 {
		t.Fatalf("the notes sender was not settled as a role mailbox (%d rows)", n)
	}
}

// A sender types its own From, so the rule reads Gmail's authentication of
// it. A forged notes mail is judged like any other, and a noise answer it
// earns does not reach the authenticated notes that follow.
func TestAForgedNotesMailIsJudgedAndItsNoiseSparesAuthenticatedNotes(t *testing.T) {
	o := setupOwed(t)
	forged := o.notesMail(t, false)

	brain := o.judgeAsNoise(t)

	if brain.calls != 1 {
		t.Fatalf("the model was asked %d times about a forged notes mail, want 1", brain.calls)
	}
	if o.visible(t, forged) {
		t.Fatal("a forged notes mail took the exemption; the From alone decided it")
	}

	authentic := o.notesMail(t, true)
	again := o.judgeAsNoise(t)

	if !o.visible(t, authentic) {
		t.Fatal("a stored noise verdict hid Google's authenticated meeting notes")
	}
	if again.calls != 0 {
		t.Fatalf("the authenticated notes mail reached the model (%d calls)", again.calls)
	}
	// Asked again rather than filed under the stored noise answer.
	o.settledAsRoleMailbox(t)
	if o.visible(t, forged) {
		t.Fatal("settling the authenticated sender released the forged mail")
	}
}

// A personal verdict destroys mail once its window closes, so a forged notes
// mail judged personal must not take the authenticated notes with it.
func TestAForgedPersonalVerdictDestroysNoAuthenticatedNotes(t *testing.T) {
	o := setupOwed(t)
	forged := o.notesMail(t, false)
	o.judgeAs(t, capture.KindPersonal)
	authentic := o.notesMail(t, true)
	pastWindow := 31 * 24 * time.Hour
	agePersonalMail(t, o.e, forged, geminiNotes, pastWindow)
	agePersonalMail(t, o.e, authentic, geminiNotes, pastWindow)

	if n := sweepPersonal(t, o.e); n != 1 {
		t.Fatalf("the personal sweep destroyed %d messages, want only the forged one", n)
	}
	if activityBody(t, o.e, authentic) == "" {
		t.Fatal("a forged personal verdict destroyed Google's authenticated meeting notes")
	}
	if activityBody(t, o.e, forged) != "" {
		t.Fatal("the forged notes mail survived its personal verdict's window")
	}
}
