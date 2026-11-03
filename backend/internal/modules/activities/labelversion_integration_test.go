// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// labelCurrent writes a verdict about the row as it stands now. It is for tests
// about what a label does, not the read-to-write window.
func labelCurrent(ctx context.Context, store *Store, id ids.UUID, label string) (bool, error) {
	var version int64
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT version FROM activity WHERE id = $1 AND restricted_at IS NULL`, id).Scan(&version)
	}); err != nil {
		return false, err
	}
	return store.SetCaptureLabel(ctx, id, label, version)
}

// A verdict is about the text the classifier read. When the body changes while
// the model call runs, the verdict must not label the new text.
func TestAVerdictAboutAnEarlierBodyDoesNotLabelTheRestoredOne(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()
	id := e.seed(t, capturedRow{kind: "email"})

	backlog, err := e.store.UnlabeledCaptureEmails(ctx, rulesetNew, 200, 200)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	var read *UnlabeledEmail
	for i := range backlog {
		if backlog[i].ID == id {
			read = &backlog[i]
		}
	}
	if read == nil {
		t.Fatal("the message was not offered to the classifier, so there is no read to go stale")
	}

	e.exec(t, `UPDATE activity SET body = 'the full restored text', capture_label = NULL WHERE id = $1`, id)

	applied, err := e.store.SetCaptureLabel(ctx, id, "commitment", read.Version)
	if err != nil {
		t.Fatalf("writing the verdict: %v", err)
	}
	if applied {
		t.Error("a verdict about the earlier body labelled the restored one, so the row leaves the backlog on text it never held")
	}
	var label *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT capture_label FROM activity WHERE id = $1`, id).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != nil {
		t.Errorf("the restored message carries label %q, want none", *label)
	}

	if applied, err := labelCurrent(ctx, e.store, id, "commitment"); err != nil || !applied {
		t.Fatalf("a verdict about the restored body = applied %v, err %v, want applied", applied, err)
	}
}
