// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The duty a new contact incurs, recorded from the person the real creation
// door wrote.
//
// It drives the consumer over a person created through POST /v1/people rather
// than a hand-inserted row: the acquisition evidence the duty is read from is
// written by that door, and a test seeding its own would prove the consumer
// works against a fixture nothing produces.

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// driveNoticeCase runs the consumer for one person, as the worker would.
func driveNoticeCase(t *testing.T, e *apptest.AppEnv, personID string) {
	t.Helper()
	id, err := ids.Parse(personID)
	if err != nil {
		t.Fatalf("parsing the person id: %v", err)
	}
	consumer := compose.NewNoticeCaseOpen(e.Pool, time.Now, slog.New(slog.DiscardHandler))
	if err := consumer.HandleEvent(context.Background(), events.Envelope{
		Type:   "person.created",
		Entity: events.EntityRef{Type: "person", ID: id},
	}); err != nil {
		t.Fatalf("recording the duty: %v", err)
	}
}

func noticeCaseFor(t *testing.T, e *apptest.AppEnv, personID string) (rule, state string, due time.Time, found bool) {
	t.Helper()
	row := e.Owner.QueryRow(context.Background(), `
		SELECT rule, state, due_at FROM privacy_notice_case WHERE person_id = $1`, personID)
	if err := row.Scan(&rule, &state, &due); err != nil {
		return "", "", time.Time{}, false
	}
	return rule, state, due, true
}

// TestABoughtContactIsOwedANotice is the duty this table exists for.
//
// Nobody told this person we hold their data — they never asked to hear from
// us — so Art. 14 gives the controller a month to say so. Before this the duty
// existed and was invisible: there was no way to ask who had not been told.
func TestABoughtContactIsOwedANotice(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	person := personAcquiredAs(t, e, "Bought Contact", "purchased_or_imported")

	driveNoticeCase(t, e, person)

	rule, state, due, found := noticeCaseFor(t, e, person)
	if !found {
		t.Fatal("a contact from a bought list is owed an Art. 14 notice and none was recorded: " +
			"the duty is real whether or not anything writes it down, and unwritten it is one " +
			"nobody can discharge")
	}
	if rule != "art14" {
		t.Errorf("rule = %q, want art14: the data came from a list, not from the subject", rule)
	}
	if state != "open" {
		t.Errorf("state = %q, want open", state)
	}
	if until := time.Until(due); until <= 0 || until > 31*24*time.Hour {
		t.Errorf("the notice falls due in %v, want inside a month of the acquisition", until)
	}
}

// TestAContactWhoWroteToUsOwesNothing is the other half of the split.
//
// They handed us the data themselves, so Art. 13 was discharged by the surface
// that took it. A case here would be a duty nobody owes, on a queue that exists
// to show real ones.
func TestAContactWhoWroteToUsOwesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	person := personAcquiredAs(t, e, "Wrote To Us", "subject_initiated")

	driveNoticeCase(t, e, person)

	if _, _, _, found := noticeCaseFor(t, e, person); found {
		t.Error("a contact who wrote to us first was recorded as owed a notice: they handed us " +
			"the data, so the disclosure was owed at collection by the surface that took it")
	}
}

// TestARedeliveredCreationRecordsOneDuty holds the idempotency the at-least-once
// bus requires. The guarantee is the unique index on acquisition_id, not the
// dedupe cache in front of it — which marks AFTER the effect, so a crash in
// that window replays and replay has to be free.
func TestARedeliveredCreationRecordsOneDuty(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	person := personAcquiredAs(t, e, "Twice Delivered", "referral")

	driveNoticeCase(t, e, person)
	driveNoticeCase(t, e, person)

	var cases int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM privacy_notice_case WHERE person_id = $1`, person).Scan(&cases); err != nil {
		t.Fatal(err)
	}
	if cases != 1 {
		t.Errorf("a redelivered person.created recorded %d duties, want 1: one acquisition owes "+
			"one disclosure, and a second row would sit on the queue forever after the first "+
			"was discharged", cases)
	}
}

// personAcquiredAs creates a person through the HTTP door and states how it was
// obtained.
//
// The acquisition is written directly because CreatePersonRequest carries no
// acquisition field: the web door cannot say how a contact arrived, so every
// person it creates is unknown_legacy. The importing doors CAN say
// (compose/csvpersonfields.go and flipwriters.go both pass
// AcquiredPurchasedOrImported), and this fixture stands in for them — the row
// it writes is the row they write, in the same shape and the same columns.
//
// Widening the create contract to carry a kind is a real gap and a separate
// change; a test that pretended the field existed would be describing a product
// we do not ship.
func personAcquiredAs(t *testing.T, e *apptest.AppEnv, name, kind string) string {
	t.Helper()
	var person struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": name, "source": "manual",
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}
	tag, err := e.Owner.Exec(context.Background(), `
		UPDATE person_acquisition_evidence SET kind = $2 WHERE person_id = $1`,
		person.ID, kind)
	if err != nil {
		t.Fatalf("stating how the contact was obtained: %v", err)
	}
	// Asserted, because a zero-row UPDATE is silent: were createPerson to stop
	// writing acquisition evidence, this fixture would quietly state nothing
	// and TestAContactWhoWroteToUsOwesNothing would still pass — finding no
	// case, for entirely the wrong reason.
	if rows := tag.RowsAffected(); rows != 1 {
		t.Fatalf("the creation door wrote %d acquisition row(s), want 1: this fixture states "+
			"how a contact arrived by editing that row, and with none there it states nothing", rows)
	}
	return person.ID
}

// TestAMergeCarriesTheDutyToTheSurvivor holds the pair that a merge splits.
//
// The merge MOVES acquisition evidence onto the survivor. A notice case is the
// duty owed for that evidence, so one left on the retired record is worse than
// stale: the case is read off the survivor's queue, so the workspace still owes
// a disclosure it can no longer see — and because the merge ARCHIVES rather
// than deletes the source, no cascade ever cleans it up. An erasure of the
// survivor would miss it too.
func TestAMergeCarriesTheDutyToTheSurvivor(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	survivor := personAcquiredAs(t, e, "Survivor", "subject_initiated")
	retired := personAcquiredAs(t, e, "Retired Duplicate", "purchased_or_imported")

	driveNoticeCase(t, e, retired)
	if _, _, _, found := noticeCaseFor(t, e, retired); !found {
		t.Fatal("the bought contact was owed a notice and none was recorded")
	}

	if status := e.Call(t, "POST", "/v1/people/"+retired+"/merge", AnyMap{
		"target_id": survivor,
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("merge → %d", status)
	}

	if _, _, _, found := noticeCaseFor(t, e, survivor); !found {
		t.Error("the duty stayed on the retired record after a merge: it is read off the " +
			"survivor's queue, so the workspace owes a disclosure it can no longer see, and " +
			"the merge archives rather than deletes so no cascade will ever clear it")
	}
	var stranded int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM privacy_notice_case WHERE person_id = $1`, retired).Scan(&stranded); err != nil {
		t.Fatal(err)
	}
	if stranded != 0 {
		t.Errorf("%d notice case(s) still name the retired record", stranded)
	}
}

// TestEveryUncasedAcquisitionGetsItsDuty covers the person holding more than
// one acquisition by the time the consumer runs.
//
// A merge MOVES evidence onto the survivor, so a contact can hold several rows
// while person.created fired only once. Handling the first would leave the rest
// owed and unrecorded forever — nothing looks at them again.
func TestEveryUncasedAcquisitionGetsItsDuty(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	person := personAcquiredAs(t, e, "Twice Acquired", "purchased_or_imported")

	// A second acquisition, as a merge would leave behind.
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO person_acquisition_evidence (person_id, kind, captured_by)
		VALUES ($1, 'referral', 'test')`, person); err != nil {
		t.Fatalf("seeding the carried acquisition: %v", err)
	}

	driveNoticeCase(t, e, person)

	var cases int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM privacy_notice_case WHERE person_id = $1`, person).Scan(&cases); err != nil {
		t.Fatal(err)
	}
	if cases != 2 {
		t.Errorf("two acquisitions produced %d duties, want 2: each is a separate disclosure "+
			"with its own deadline, and one left uncased is owed forever with nothing "+
			"scheduled to look at it again", cases)
	}
}
