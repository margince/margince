// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The reminder for a judged request says who asked and what, in their words.
// The ask is kept with the verdict, so any later pass that files the reminder
// quotes it, whichever pass judged the mail.
func TestARequestReminderQuotesWhatItsSenderAsked(t *testing.T) {
	t.Parallel()
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

// A request whose row another writer holds is skipped by that pass, and the
// next pass still quotes the ask the verdict kept.
func TestARequestSkippedWhileLockedIsQuotedByTheNextPass(t *testing.T) {
	t.Parallel()
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if _, err := store.SetOwedVerdictAsked(asClassifier(e), source, OwedVerdictAsksUs, "prompts-ask", dbNow(t, e), "Please send the report."); err != nil {
		t.Fatal(err)
	}
	holder, err := e.owner.Begin(e.as())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(e.as(), `SELECT 1 FROM activity WHERE id = $1 FOR UPDATE`, source); err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := holder.Rollback(e.as()); err != nil {
		t.Fatal(err)
	}
	var filed int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&filed); err != nil {
		t.Fatal(err)
	}
	if filed != 0 {
		t.Fatalf("%d reminder(s) filed for a row another writer held, want the pass to skip it", filed)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, want := reminderBody(t, e, store, source), "“Please send the report.”"; !strings.Contains(got, want) {
		t.Fatalf("reminder body %q, want it to quote %q", got, want)
	}
}

// A filing that fails rolls back only the reminder; the ask stays with the
// verdict, so the retry still quotes it.
func TestARequestWhoseFilingFailedIsQuotedByTheRetry(t *testing.T) {
	t.Parallel()
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if _, err := store.SetOwedVerdictAsked(asClassifier(e), source, OwedVerdictAsksUs, "prompts-ask", dbNow(t, e), "Please send the report."); err != nil {
		t.Fatal(err)
	}
	refuse := "refuse_reminder_" + strings.ReplaceAll(source.String(), "-", "")
	e.exec(t, `CREATE FUNCTION `+refuse+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.source_activity_id = '`+source.String()+`' THEN RAISE EXCEPTION 'filing refused'; END IF; RETURN NEW; END $$`)
	e.exec(t, `CREATE TRIGGER `+refuse+` BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION `+refuse+`()`)
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+refuse+` ON activity; DROP FUNCTION IF EXISTS `+refuse+`()`); err != nil {
			t.Errorf("removing the refusing trigger: %v", err)
		}
	})
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err == nil {
		t.Fatal("the refused filing reported success")
	}
	e.exec(t, `DROP TRIGGER `+refuse+` ON activity`)
	e.exec(t, `DROP FUNCTION `+refuse+`()`)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, want := reminderBody(t, e, store, source), "“Please send the report.”"; !strings.Contains(got, want) {
		t.Fatalf("reminder body %q, want it to quote %q", got, want)
	}
}

// A later verdict without an ask forgets the earlier one, so a reminder never
// quotes a sentence the current verdict did not keep.
func TestAVerdictWithoutAnAskClearsTheOldOne(t *testing.T) {
	t.Parallel()
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
