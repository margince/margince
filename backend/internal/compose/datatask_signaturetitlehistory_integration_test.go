// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// oneMailWatermark records the read the one-mail pass made of the newest mail:
// a watermark on it and no history depth.
func (h *historyEnv) oneMailWatermark(t *testing.T, contact, mail ids.UUID) {
	t.Helper()
	h.e.WsExec(t, `INSERT INTO contact_signature_enrich_state (contact_id, activity_id, last_activity_at)
		SELECT $1, id, occurred_at FROM activity WHERE id = $2`, contact, mail)
}

func (h *historyEnv) hasWatermark(t *testing.T, contact ids.UUID) bool {
	t.Helper()
	return h.count(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id = $1`, contact) == 1
}

func runSignatureHistoryBatch(ctx context.Context, h *historyEnv, in datatask.BatchIn) (datatask.BatchOut, error) {
	var out datatask.BatchOut
	err := database.WithWorkspaceTx(ctx, h.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = signatureTitleHistoryDataTask(h.e.Pool).Steps[0].Batch(ctx, tx, in)
		return err
	})
	return out, err
}

// writer is a contact who sent the bodies in order, one day apart, and whose
// newest mail carries the watermark the one-mail pass set.
func (h *historyEnv) writer(t *testing.T, name, address string, bodies ...string) ids.UUID {
	t.Helper()
	contact := h.owedEnv.contact(t, name, address)
	var newest ids.UUID
	for day, body := range bodies {
		newest = h.capture(t, owedMail{
			from: address, to: h.seat, subject: "Order", body: body,
			at: h.day.AddDate(0, 0, day), messageID: ids.NewV7().String() + "@customer.example",
		})
	}
	h.oneMailWatermark(t, contact, newest)
	return contact
}

// inspectedByTheRepair runs every batch at limit 1 on one transaction it rolls
// back, and returns the rows the batches inspected and the estimate before them.
func inspectedByTheRepair(ctx context.Context, t *testing.T, h *historyEnv) (inspected, estimated int64) {
	t.Helper()
	rollback := errors.New("rolled back")
	step := signatureTitleHistoryDataTask(h.e.Pool).Steps[0]
	err := database.WithWorkspaceTx(ctx, h.e.Pool, func(tx pgx.Tx) error {
		var err error
		if estimated, _, err = step.Estimate(ctx, tx, 1000); err != nil {
			return err
		}
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		for {
			out, err := step.Batch(ctx, tx, in)
			if err != nil {
				return err
			}
			inspected += int64(out.Scanned)
			if out.Done {
				return rollback
			}
			in.Cursor = out.Cursor
		}
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("running the repair batches: %v", err)
	}
	return inspected, estimated
}

func TestTheSignatureHistoryTaskReopensOnlyUntitledContactsWithOlderMail(t *testing.T) {
	h := setupHistory(t)
	ctx := principal.SystemActing(h.e.Admin(), "system:datatask-test")
	older := h.writes(t, 0, signed("Head of Procurement"))
	newest := h.writes(t, 3, unsigned)
	h.oneMailWatermark(t, h.contact, newest)
	h.brain.answers[older] = statesTitle("Head of Procurement")

	titled := h.writer(t, "Lee Human", "lee@customer.example", "Lee Human\nIntern", unsigned)
	if _, err := h.e.Contacts.UpdateContact(h.reader(), ids.From[ids.ContactKind](titled),
		contacts.UpdateContactInput{Title: new("Head of Sales")}); err != nil {
		t.Fatal(err)
	}
	cleared := h.writer(t, "Mo Cleared", "mo@customer.example", "Mo Cleared\nIntern", unsigned)
	h.entersThenClears(t, cleared)
	single := h.writer(t, "Kim Single", "kim@customer.example", unsigned)
	plain := h.writer(t, "Jo Plain", "jo@customer.example", unsigned, unsigned)
	current := h.owedEnv.contact(t, "Ana Current", "ana@customer.example")
	h.e.WsExec(t, `INSERT INTO contact_signature_enrich_state (contact_id, activity_id, last_activity_at, history_depth)
		SELECT $1, id, occurred_at, 3 FROM activity WHERE id = $2`, current, newest)

	if _, err := runSignatureHistoryBatch(ctx, h, datatask.BatchIn{Cursor: json.RawMessage(`{`), Limit: 10, RunID: ids.NewV7()}); err == nil {
		t.Fatal("a malformed cursor was accepted")
	}
	if inspected, estimated := inspectedByTheRepair(ctx, t, h); inspected != estimated || inspected != 5 {
		t.Fatalf("the repair inspected %d watermarks and estimated %d, want the 5 one-mail watermarks both times", inspected, estimated)
	}
	if !h.hasWatermark(t, h.contact) {
		t.Fatal("a rolled-back batch removed the watermark")
	}

	task := signatureTitleHistoryDataTask(h.e.Pool)
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, h.e, task)

	if h.hasWatermark(t, h.contact) || h.hasWatermark(t, plain) {
		t.Fatal("an untitled contact with older mail kept the one-mail watermark")
	}
	for name, kept := range map[string]ids.UUID{"a typed title": titled, "a cleared title": cleared, "no other mail": single, "a current watermark": current} {
		if !h.hasWatermark(t, kept) {
			t.Errorf("the task reopened a contact with %s", name)
		}
	}

	h.pass(t)

	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q after the repair and one pass, want the older signature's", got)
	}
	if got := h.e.WsScalar(t, `SELECT title FROM contact WHERE id = $1`, titled); got != "Head of Sales" {
		t.Fatalf("the human-entered title became %q", got)
	}
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	for {
		out, err := runSignatureHistoryBatch(ctx, h, in)
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a second run reopened %d watermarks", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}
