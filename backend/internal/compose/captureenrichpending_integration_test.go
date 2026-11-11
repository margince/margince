// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (h *historyEnv) queueSignature(t *testing.T, contact ids.UUID) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), h.e.WS), "agent:enrich")
	if err := database.WithWorkspaceTx(ctx, h.e.Pool, func(tx pgx.Tx) error {
		return contacts.QueueSignatureWorkTx(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, nil)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPendingSignaturePassReadsOnlyItsRequestedContacts(t *testing.T) {
	h := setupHistory(t)
	mail := h.writes(t, 0, signed("Head of Procurement"))
	h.brain.answers[mail] = statesTitle("Head of Procurement")
	unrelated := h.owedEnv.contact(t, "Unrequested correspondent", "other@customer.example")
	h.capture(t, owedMail{from: "other@customer.example", to: h.seat, subject: "Another order", body: signed("Director"), at: h.day, messageID: ids.NewV7().String() + "@customer.example"})
	h.queueSignature(t, h.contact)
	reader := NewCaptureEnricher(h.e.Pool, h.brain, slog.New(slog.DiscardHandler))
	ctx := principal.WithWorkspaceID(context.Background(), h.e.WS)
	if _, err := reader.RunPendingWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title=%q", got)
	}
	var attempts int
	if err := database.WithWorkspaceTx(ctx, h.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact`, pgx.NamedArgs{"contact": unrelated}).Scan(&attempts)
	}); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("unrequested contact was read: %d attempts", attempts)
	}
	if _, err := reader.RunPendingWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{mail}) {
		t.Fatalf("model requests=%v", got)
	}
}

func TestPendingSignaturePassKeepsARequestArrivingDuringTheModelRead(t *testing.T) {
	h := setupHistory(t)
	first := h.writes(t, 0, signed("Head of Procurement"))
	h.brain.answers[first] = statesTitle("Head of Procurement")
	h.queueSignature(t, h.contact)
	h.brain.onAsk = func(ids.UUID) {
		system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), h.e.WS), "agent:enrich")
		if err := database.WithWorkspaceTx(system, h.e.Pool, func(tx pgx.Tx) error {
			_, err := contacts.QueueSignatureEventTx(system, tx, []ids.ContactID{ids.From[ids.ContactKind](h.contact)}, contacts.SignatureRequest{Reopen: true, Live: true})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewCaptureEnricher(h.e.Pool, h.brain, slog.New(slog.DiscardHandler))
	ctx := principal.WithWorkspaceID(context.Background(), h.e.WS)
	if _, err := reader.RunPendingWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	store := newContactsStore(InstallationDB(h.e.Pool))
	work, err := store.PendingSignatureWork(principal.SystemActing(ctx, "agent:enrich"), 10)
	if err != nil || len(work) != 1 {
		t.Fatalf("successor work=%+v err=%v", work, err)
	}
	if got := h.count(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND activity_id IS NULL`, pgx.NamedArgs{"contact": h.contact}); got != 1 {
		t.Fatal("stale model completion overwrote a newer invalidation")
	}
	h.brain.onAsk = nil
	if _, err := reader.RunPendingWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	work, err = store.PendingSignatureWork(principal.SystemActing(ctx, "agent:enrich"), 10)
	if err != nil || len(work) != 0 {
		t.Fatalf("settled successor=%+v err=%v", work, err)
	}
}

func TestDeletingAnUnreadSignatureSourceKeepsTheContactPending(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Director"))
	newest := h.writes(t, 1, signed("Head of Procurement"))
	h.queueSignature(t, h.contact)
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), h.e.WS), "agent:enrich")
	store := newContactsStore(InstallationDB(h.e.Pool))
	work, err := store.PendingSignatureWork(ctx, 100)
	if err != nil || len(work) != 1 {
		t.Fatalf("selected work=%+v err=%v", work, err)
	}
	if err := database.WithWorkspaceTx(ctx, h.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM activity WHERE id=@id`, pgx.NamedArgs{"id": newest})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSignatureRead(ctx, ids.From[ids.ContactKind](h.contact), newest, 3, contacts.SignatureRead{}, work[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.SettleSignatureWork(ctx, work[0], 0); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingSignatureWork(ctx, 100)
	if err != nil || len(pending) != 1 || pending[0].Generation <= work[0].Generation {
		t.Fatalf("source deletion was lost: %+v err=%v", pending, err)
	}
	candidates, err := store.SignatureCandidatesForContacts(ctx, []ids.ContactID{ids.From[ids.ContactKind](h.contact)}, 100, true)
	if err != nil || len(candidates) != 1 || candidates[0].ActivityID != older {
		t.Fatalf("remaining sources=%+v err=%v", candidates, err)
	}
}

func TestSharedSignatureDeferralSnoozesTheWholePendingPass(t *testing.T) {
	for _, delay := range []time.Duration{3 * time.Minute, 30 * 24 * time.Hour} {
		t.Run(delay.String(), func(t *testing.T) {
			h := setupHistory(t)
			mail := h.writes(t, 0, signed("Director"))
			now := clocktest.Now(t)
			h.brain.fail[mail] = &ai.BudgetDeferralError{Task: ai.TaskEnrich, NextAttemptAt: now.Add(delay)}
			h.queueSignature(t, h.contact)
			reader := NewCaptureEnricher(h.e.Pool, h.brain, slog.New(slog.DiscardHandler))
			reader.now = func() time.Time { return now }
			worker := &captureEnrichWorker{pool: h.e.Pool, enricher: reader, log: reader.log}
			err := worker.Work(context.Background(), &river.Job[CaptureEnrichArgs]{Args: CaptureEnrichArgs{Pending: true}})
			snooze, ok := errors.AsType[*river.JobSnoozeError](err)
			if !ok || snooze.Duration != min(delay, 15*time.Minute) {
				t.Fatalf("deferral=%v want snooze %v", err, min(delay, 15*time.Minute))
			}
			if got := h.count(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND requested_generation>processed_generation`, pgx.NamedArgs{"contact": h.contact}); got != 1 {
				t.Fatalf("deferred work was acknowledged: %d", got)
			}
		})
	}
}
