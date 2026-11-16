// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The reminder for a judged request says who asked and what, in their words.
// The ask is kept with the verdict, so any later pass that files the reminder
// quotes it, whichever pass judged the mail.
func TestARequestReminderQuotesWhatItsSenderAsked(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address, display_name) VALUES ($1, 'from', $2, 'Anna Weber')`, source, requestCounterparty)
	store := storeKnowing(e)
	if applied, err := store.SetOwedVerdictAsked(asClassifier(e), source, OwedVerdictAsksUs, "prompts-ask", dbNow(t, e), "Please send the report."); err != nil || !applied {
		t.Fatalf("verdict applied %v, err %v", applied, err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want := "Anna Weber asked: “Please send the report.”"
	if got := reminderBody(t, e, store, source); got != want {
		t.Fatalf("reminder body %q, want %q", got, want)
	}
}

// A later verdict without an ask forgets the earlier one, so a reminder never
// quotes a sentence the current verdict did not keep.
func TestAVerdictWithoutAnAskClearsTheOldOne(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if _, err := store.SetOwedVerdictAsked(asClassifier(e), source, OwedVerdictAsksUs, "prompts-ask", dbNow(t, e), "Please send the report."); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetOwedVerdict(asClassifier(e), source, OwedVerdictAsksUs, "prompts-later", dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	var kept *string
	if err := e.owner.QueryRow(e.as(), `SELECT owed_ask FROM activity WHERE id = $1`, source).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != nil {
		t.Fatalf("owed_ask = %q after a verdict with no ask, want none", *kept)
	}
}

func reminderBody(t *testing.T, e *loadEnv, store *Store, source ids.UUID) string {
	t.Helper()
	var task ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&task); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body == nil {
		return ""
	}
	return *got.Body
}
