// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func beginUnavailableThread(t *testing.T, e *integration.Env, key string) ids.UUID {
	t.Helper()
	run := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capture.QueueThreadRepairTx(e.Admin(), tx, key)
	}); err != nil {
		t.Fatal(err)
	}
	for range 1000 {
		available := true
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(e.Admin(), tx, 1, run, threadRepairApplier(e.Pool)); err != nil {
				return err
			}
			return tx.QueryRow(e.Admin(), `SELECT activity_thread_available(@key)`, pgx.NamedArgs{"key": key}).Scan(&available)
		}); err != nil {
			t.Fatal(err)
		}
		if !available {
			return run
		}
	}
	t.Fatal("repair never established its conversation barrier")
	return ids.Nil
}

func finishUnavailableThread(t *testing.T, e *integration.Env, run ids.UUID) {
	t.Helper()
	for range 4000 {
		done := false
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			scanned, _, finished, err := capture.ThreadRepairBatchTx(e.Admin(), tx, 1, run, threadRepairApplier(e.Pool))
			if scanned > 1 {
				t.Fatalf("limit-one batch inspected %d operations", scanned)
			}
			done = finished
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if done {
			if failures := e.WsCount(t, `SELECT count(*) FROM capture_thread_repair WHERE state<>'ready'`); failures != 0 {
				t.Fatalf("repair drain left %d failed components", failures)
			}
			return
		}
	}
	t.Fatal("repair never published the complete conversation")
}

func TestThreadRepairWithholdsOrdinaryActivityAccessAndPendingDelivery(t *testing.T) {
	f := newThreadOwnerFixture(t)
	ctx := f.As(f.Rep1, []ids.UUID{f.Team1}, integration.AdminPerms)
	run := beginUnavailableThread(t, f.Env, f.source)
	if _, err := f.Activities.GetActivity(ctx, f.first, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("get during repair: %v", err)
	}
	rows, _, err := f.Activities.ListActivities(ctx, activities.ListActivitiesInput{ThreadKey: &f.source})
	if err != nil || len(rows) != 0 {
		t.Fatalf("list during repair returned %d rows: %v", len(rows), err)
	}
	body := "A reviewed update."
	if _, _, err := f.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Body: &body, Source: "manual", ThreadKey: f.source}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("create on unavailable thread: %v", err)
	}
	if err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error { return activities.LockActivityLive(ctx, tx, f.first) }); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("lock unavailable activity: %v", err)
	}
	if _, err := f.Activities.UpdateActivity(ctx, f.first, activities.UpdateActivityInput{Body: &body}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("update during repair: %v", err)
	}
	delivery := comms.NewStore(f.DB(), func() time.Time { return time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC) }, nil)
	if _, err := delivery.Load(f.Admin(), f.delivery); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("pending delivery during repair must retry: %v", err)
	}
	if got := f.WsCount(t, `SELECT attempts FROM comms_outbound WHERE id=$1 AND status='pending'`, f.delivery); got != 0 {
		t.Fatalf("blocked delivery spent %d attempts", got)
	}
	finishUnavailableThread(t, f.Env, run)
	if _, err := f.Activities.GetActivity(ctx, f.first, storekit.LiveOnly); err != nil {
		t.Fatal(err)
	}
	rows, _, err = f.Activities.ListActivities(ctx, activities.ListActivitiesInput{ThreadKey: &f.source})
	if err != nil || len(rows) != 1 {
		t.Fatalf("published source list returned %d rows: %v", len(rows), err)
	}
	rows, _, err = f.Activities.ListActivities(ctx, activities.ListActivitiesInput{ThreadKey: &f.destination})
	if err != nil || len(rows) != 1 {
		t.Fatalf("published destination list returned %d rows: %v", len(rows), err)
	}
	if _, err := f.Activities.UpdateActivity(ctx, f.first, activities.UpdateActivityInput{Body: &body}); err != nil {
		t.Fatal(err)
	}
	loaded, err := delivery.Load(f.Admin(), f.delivery)
	if err != nil || loaded.ActivityID != f.second || loaded.Attempts != 1 {
		t.Fatalf("published delivery = %+v: %v", loaded, err)
	}
}

func TestThreadRepairWithholdsReplyAndAnsweredEvidenceUntilPublication(t *testing.T) {
	e := setupVoiceSend(t)
	address := seatAddress(t, e.Env, e.Rep1)
	key := "availability-parent@buyer.test"
	raw := strings.ReplaceAll(string(subjectThreadMail(address, key, "Pricing", "")), "pat@counterparty.example", e.recipient)
	captureThreadMail(t, e.Env, e.Rep1, []byte(raw))
	parent := capturedThreadActivity(t, e.Env, key)
	stager := &recordingStager{}
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	sender := sendStore(e.Pool, SendPath{PublicBaseURL: voiceSendBaseURL, Delivery: stager}).WithClock(func() time.Time { return now })
	input := activities.SendEmailInput{Recipients: []string{e.recipient}, Subject: "Pricing", Body: "Here is the requested answer.", ConsentPurpose: "transactional"}
	sent, err := sender.SendEmail(e.ctx, activities.FromActivity(parent), input, consent.NewGate(consent.NewStore(e.DB())), stager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.UpdateActivity(e.ctx, ids.From[ids.ActivityKind](ids.UUID(sent.Id)), activities.UpdateActivityInput{OccurredAt: &now}); err != nil {
		t.Fatal(err)
	}
	assertThreadAnswered(t, e.Env, parent, now, true)
	run := beginUnavailableThread(t, e.Env, key)
	assertThreadAnswered(t, e.Env, parent, now, false)
	if _, err := sender.SendEmail(e.ctx, activities.FromActivity(parent), input, consent.NewGate(consent.NewStore(e.DB())), stager); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reply during repair: %v", err)
	}
	brain := &replyBrainStub{response: model.Response{Text: `{"body":"Thanks for the details."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := drafter.DraftEmailWithProvenance(e.ctx, parent.UUID, draftfloor.Steering{Intent: "Confirm the details"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("draft during repair: %v", err)
	}
	if len(brain.request.Messages) != 0 {
		t.Fatal("unavailable source reached the reply model")
	}
	finishUnavailableThread(t, e.Env, run)
	assertThreadAnswered(t, e.Env, parent, now, true)
	if _, err := sender.SendEmail(e.ctx, activities.FromActivity(parent), input, consent.NewGate(consent.NewStore(e.DB())), stager); err != nil {
		t.Fatal(err)
	}
}

func assertThreadAnswered(t *testing.T, e *integration.Env, id ids.ActivityID, until time.Time, want bool) {
	t.Helper()
	var answered bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		bound := fmt.Sprintf("$%d::timestamptz", arg(until))
		source := fmt.Sprintf("$%d::uuid", arg(id))
		query := `SELECT ` + storekit.AnsweredSQL("inbound", bound) + ` FROM activity inbound WHERE inbound.id=` + source
		return tx.QueryRow(e.Admin(), query, args...).Scan(&answered)
	}); err != nil {
		t.Fatal(err)
	}
	if answered != want {
		t.Fatalf("answered=%v want=%v", answered, want)
	}
}

func TestThreadRepairWithholdsCurrentCommitmentEvidenceUntilPublication(t *testing.T) {
	f := setupRenewalPrivacy(t, .95)
	assertCurrentThreadCommitment(t, f, true)
	run := beginUnavailableThread(t, f.e, "renewal-privacy")
	assertCurrentThreadCommitment(t, f, false)
	finishUnavailableThread(t, f.e, run)
	assertCurrentThreadCommitment(t, f, true)
}

func assertCurrentThreadCommitment(t *testing.T, f renewalPrivacyFixture, want bool) {
	t.Helper()
	var current bool
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		source := fmt.Sprintf("$%d::uuid", arg(f.messages[0]))
		locator := fmt.Sprintf("$%d::text", arg(f.commitment.Locator))
		query := `SELECT ` + provenance.CurrentCommitmentClause(source, locator, "NULL::timestamptz", f.commitment.AsOf, arg)
		return tx.QueryRow(context.Background(), query, args...).Scan(&current)
	}); err != nil {
		t.Fatal(err)
	}
	if current != want {
		t.Fatalf("current commitment=%v want=%v", current, want)
	}
}
