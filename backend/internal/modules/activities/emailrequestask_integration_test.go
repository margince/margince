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
func TestARequestReminderQuotesWhatItsSenderAsked(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address, display_name) VALUES ($1, 'from', $2, 'Anna Weber')`, source, requestCounterparty)
	store := storeKnowing(e)
	asks := RequestAsks{}
	asks.Keep(source, true, "Please send the report.")
	if err := store.FileAskedRequests(asClassifier(e), requestInstant.Add(time.Hour), asks); err != nil {
		t.Fatal(err)
	}
	var task ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&task); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	want := "Anna Weber asked: “Please send the report.”"
	if got.Body == nil {
		t.Fatalf("reminder has no body, want %q", want)
	}
	if *got.Body != want {
		t.Fatalf("reminder body %q, want %q", *got.Body, want)
	}
}

// Filing the asked reminders reads only the asked messages. Other waiting
// mail is left for the full pass.
func TestFilingAskedRemindersLeavesOtherRequestsToTheFullPass(t *testing.T) {
	e := setupLoad(t)
	asked := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	other := seedEmailRequest(t, e, "Pricing sheet", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	asks := RequestAsks{}
	asks.Keep(asked, true, "Please send the report.")
	if err := store.FileAskedRequests(asClassifier(e), requestInstant.Add(time.Hour), asks); err != nil {
		t.Fatal(err)
	}
	filed := func(source ids.UUID) bool {
		var n int
		if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if !filed(asked) || filed(other) {
		t.Fatalf("asked filed %v, other filed %v; want only the asked one", filed(asked), filed(other))
	}
	if err := store.FileAskedRequests(asClassifier(e), requestInstant.Add(time.Hour), RequestAsks{}); err != nil {
		t.Fatal(err)
	}
	if filed(other) {
		t.Fatal("an empty set of asks filed a reminder")
	}
}
