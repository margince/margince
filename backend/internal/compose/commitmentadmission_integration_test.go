// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCommitmentAdmissionUsesCurrentEvidenceBeforeWriting(t *testing.T) {
	asOf := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		source time.Time
		due    string
		later  bool
		kind   string
		status *string
		want   CommitmentOutcome
	}{
		{"old undated", asOf.AddDate(-2, 0, 0), "", false, "meeting", nil, CommitmentOutcome(activities.CommitmentHistorical)},
		{"old but due next month", asOf.AddDate(-2, 0, 0), "2026-11-18", false, "email", nil, CommitmentTaskWritten},
		{"recent overdue deadline", asOf.AddDate(0, 0, -45), "2026-10-15", false, "email", nil, CommitmentTaskWritten},
		{"recent", asOf.AddDate(0, 0, -2), "", false, "email", nil, CommitmentTaskWritten},
		{"unread later context", asOf.AddDate(0, 0, -2), "2026-11-18", true, "email", nil, CommitmentOutcome(activities.CommitmentIncomplete)},
		{"cancelled future meeting", asOf.AddDate(0, 0, 2), "2026-11-18", false, "meeting", new("canceled"), CommitmentOutcome(activities.CommitmentMeetingCancelled)},
		{"cancelled past meeting", asOf.AddDate(0, 0, -2), "", false, "meeting", new("canceled"), CommitmentOutcome(activities.CommitmentMeetingCancelled)},
		{"future booked meeting", asOf.AddDate(0, 0, 2), "", false, "meeting", new("booked"), CommitmentOutcome(activities.CommitmentMeetingPending)},
		{"promise from already held meeting", asOf.AddDate(0, 0, -2), "", false, "meeting", new("held"), CommitmentTaskWritten},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			e := integration.Setup(t)
			body, key := "I will send the pricing.", "pricing@buyer.test"
			direction := "outbound"
			// A colleague's own late report is proposed; the horizon's history is for imported evidence.
			logged := "manual"
			if test.want == CommitmentOutcome(activities.CommitmentHistorical) {
				logged = "import"
			}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: test.kind, Body: &body, Direction: &direction, OccurredAt: &test.source,
				ThreadKey: key, Source: logged, MeetingStatus: test.status,
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.later {
				at, answer := asOf.AddDate(0, 0, -1), "Here is the pricing."
				if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Body: &answer, Direction: &direction, OccurredAt: &at, ThreadKey: key, Source: "manual"}); err != nil {
					t.Fatal(err)
				}
			}
			sourceID := ids.UUID(source.Id)
			c := Commitment{
				Meaning: openCommitmentMeaning(test.due, body), AsOf: asOf, ReadMessages: []ids.UUID{sourceID}, Extractor: signalScanActor, SourceActivityID: sourceID, Summary: "Send the pricing", Quote: body, Seat: &e.Rep1, DueDate: test.due, Confidence: 0.95, Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), body), Body: body,
			}
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
			dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
			var taskID ids.UUID
			for range 2 {
				var outcome CommitmentOutcome
				err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					var err error
					var produced ids.UUID
					outcome, produced, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
					if !produced.IsZero() {
						taskID = produced
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				if outcome != test.want && (test.want != CommitmentTaskWritten || outcome != CommitmentRemembered) {
					t.Fatalf("outcome = %q, want %q", outcome, test.want)
				}
			}
			read, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](sourceID), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if read.CommitmentAdmissions == nil || len(*read.CommitmentAdmissions) != 1 {
				t.Fatalf("replay must retain one readable decision: %#v", read.CommitmentAdmissions)
			}
			decision := (*read.CommitmentAdmissions)[0]
			if decision.Outcome != string(test.want) || decision.RuleVersion != activities.CommitmentAdmissionVersion || decision.Locator != c.Locator {
				t.Fatalf("stored decision = %#v, want %q with current version and locator", decision, test.want)
			}
			wantTasks := 0
			if test.want == CommitmentTaskWritten {
				wantTasks = 1
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != wantTasks {
				t.Fatalf("tasks = %d, want %d", got, wantTasks)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval`); got != 0 {
				t.Fatalf("unexpected approvals: %d", got)
			}
			if test.name == "recent" {
				task := ids.From[ids.ActivityKind](taskID)
				for _, done := range []bool{true, false} {
					if _, err := e.Activities.UpdateActivity(e.Admin(), task, activities.UpdateActivityInput{IsDone: &done, Subject: new("My revised follow-up")}); err != nil {
						t.Fatal(err)
					}
				}
				c.AsOf = asOf.AddDate(2, 0, 0)
				if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
					if err == nil && outcome != CommitmentRemembered {
						t.Fatalf("human work re-evaluated as %q", outcome)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND NOT is_done AND archived_at IS NULL AND subject = 'My revised follow-up'`, taskID); got != 1 {
					t.Fatal("the human-reopened task changed")
				}
			}
		})
	}
}

func TestCapturedMailWithoutASentDateDoesNotCreateFreshWork(t *testing.T) {
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	raw := threadMail(address, "undated@counterparty.test", "Sun, 18 Oct 2026 10:00:00 +0000", "")
	raw = []byte(strings.ReplaceAll(string(raw), "Date: Sun, 18 Oct 2026 10:00:00 +0000\r\n", ""))
	captureThreadMail(t, e, e.Rep1, raw)
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var source ids.UUID
		if err := tx.QueryRow(ctx, `SELECT activity_id FROM activity_identity WHERE identity_kind = 'mail' AND identity_key = $1`, "undated@counterparty.test").Scan(&source); err != nil {
			return err
		}
		c := Commitment{
			Meaning: openCommitmentMeaning("", ""), AsOf: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), SourceActivityID: source, ReadMessages: []ids.UUID{source}, Extractor: signalScanActor, Locator: commitmentLocator(source, claimKindOurs, e.Rep1.String(), "Send pricing"), Seat: &e.Rep1, Summary: "Send pricing", Confidence: 0.95,
		}
		outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		if err == nil && outcome != CommitmentOutcome(activities.CommitmentSourceUnknown) {
			t.Errorf("outcome = %q, want unknown source date", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 0 {
		t.Fatalf("undated mail created %d tasks", got)
	}
}

func TestHistoricalPromiseRemainsSearchableWithoutBecomingCurrentWork(t *testing.T) {
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Pat Buyer"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2024, 10, 18, 12, 0, 0, 0, time.UTC)
	quote := "I will send the pricing."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &at, Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	c := Commitment{
		Meaning: openCommitmentMeaning("2024-10-20", quote), AsOf: at.AddDate(2, 0, 0), Extractor: signalScanActor, SourceActivityID: sourceID, Summary: "Send pricing", Quote: quote, PromisedTo: &contactID, Seat: &e.Rep1, DueDate: "2024-10-20", Confidence: 0.95, Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), quote),
	}
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	for range 2 {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
			if err == nil && outcome != CommitmentOutcome(activities.CommitmentHistorical) {
				t.Errorf("historical promise = %q", outcome)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		history, err := e.Contacts.ClaimsForContact(e.Admin(), tx, contactID, nil, 20)
		if err != nil {
			return err
		}
		if len(history) != 1 || history[0].SourceQuote != quote {
			t.Fatalf("historical claim is not readable: %#v", history)
		}
		current, err := e.Contacts.OpenCommitmentsForContact(e.Admin(), tx, contactID, nil, 20)
		if err == nil && len(current) != 0 {
			t.Errorf("history became %d current promises", len(current))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatalf("history created %d tasks", got)
	}
}

func TestProviderArrivalContradictionRemainsReviewable(t *testing.T) {
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	raw := threadMail(address, "conflicting-date@counterparty.test", "Fri, 18 Oct 2024 10:00:00 +0000", "")
	message, err := mailmap.Parse(raw, address)
	if err != nil {
		t.Fatal(err)
	}
	record := message.ToRecord("gmail", raw)
	asOf := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	record.ProviderReceivedAt = asOf.Add(-time.Hour)
	source, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(threadConnectorCtx(e, e.Rep1), record)
	if err != nil {
		t.Fatal(err)
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	c := Commitment{
		Meaning: openCommitmentMeaning("", ""),
		AsOf:    asOf, SourceActivityID: source.ID, ReadMessages: []ids.UUID{source.ID}, Extractor: signalScanActor,
		Locator: commitmentLocator(source.ID, claimKindOurs, e.Rep1.String(), "Send pricing"), Seat: &e.Rep1, Summary: "Send pricing", Confidence: 0.95,
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		if err == nil && outcome != CommitmentOutcome(activities.CommitmentTimeContradicted) {
			t.Errorf("outcome = %q", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read, err := e.Activities.GetActivity(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ActivityKind](source.ID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CommitmentAdmissions == nil || len(*read.CommitmentAdmissions) != 1 || (*read.CommitmentAdmissions)[0].Outcome != string(activities.CommitmentTimeContradicted) {
		t.Fatalf("missing timing review reason: %#v", read.CommitmentAdmissions)
	}
	if read.OccurredAt.Year() != 2024 {
		t.Fatalf("arrival replaced occurrence: %s", read.OccurredAt)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 0 {
		t.Fatalf("contradictory time created %d tasks", got)
	}
}

func TestAdmissionDoesNotDemoteWatchedOrProposedCommitments(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		theirs, legacy bool
	}{{"customer watch", true, false}, {"pending proposal", false, false}, {"legacy customer watch", true, true}, {"legacy pending proposal", false, true}} {
		theirs := scenario.theirs
		t.Run(scenario.name, func(t *testing.T) {
			e := integration.Setup(t)
			contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Pat Buyer"})
			if err != nil {
				t.Fatal(err)
			}
			contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
			at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			quote := "I will send the pricing."
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &at, Source: "import"})
			if err != nil {
				t.Fatal(err)
			}
			sourceID := ids.UUID(source.Id)
			c := Commitment{
				Meaning: openCommitmentMeaning("", quote),
				AsOf:    at.Add(time.Hour), Extractor: transcriptProposalActor, SourceActivityID: sourceID,
				Summary: "Send pricing", Quote: quote, Confidence: 0.7, Evidence: approvals.Evidence{Snippet: quote, SourceType: "activity", SourceID: sourceID}, Locator: commitmentLocator(sourceID, claimKindOurs, "Pat", quote),
			}
			want := CommitmentProposed
			if theirs {
				c.Theirs = &contactID
				want = CommitmentWatched
			} else {
				c.Seat, c.PromisedTo = &e.Rep1, &contactID
			}
			dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
			for pass := range 2 {
				if pass == 1 {
					c.AsOf = at.AddDate(0, 0, 40)
					want = CommitmentRemembered
					if scenario.legacy {
						want = CommitmentRetained
					}
				}
				if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					dispatch := dispatcher.DispatchTx
					if scenario.legacy && pass == 0 {
						dispatch = dispatcher.routeCommitment
					}
					outcome, _, err := dispatch(ctx, tx, c, ids.NewV7())
					if err == nil && outcome != want {
						t.Fatalf("pass %d = %s, want %s", pass, outcome, want)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			read, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](sourceID), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if read.CommitmentAdmissions == nil || len(*read.CommitmentAdmissions) != 1 || !(*read.CommitmentAdmissions)[0].CurrentWork {
				t.Fatalf("admitted commitment was demoted: %#v", read.CommitmentAdmissions)
			}
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				open, err := e.Contacts.ClaimsForContact(e.Admin(), tx, contactID, nil, 20)
				if err == nil && (len(open) != 1 || string(open[0].Status) != "open") {
					t.Fatalf("retained commitments = %#v", open)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'commitment_admission' AND (after::text LIKE '%source_quote%' OR before::text LIKE '%source_quote%')`, sourceID); got != 0 {
				t.Fatalf("%d audit entries retained source words", got)
			}
		})
	}
}

func TestCapturedSourceDateSurvivesOriginalRetention(t *testing.T) {
	for _, dated := range []bool{true, false} {
		t.Run(map[bool]string{true: "dated", false: "undated"}[dated], func(t *testing.T) {
			e := integration.Setup(t)
			raw := threadMail(seatAddress(t, e, e.Rep1), "retained-date@buyer.test", "Sun, 18 Oct 2026 10:00:00 +0000", "")
			if !dated {
				raw = []byte(strings.ReplaceAll(string(raw), "Date: Sun, 18 Oct 2026 10:00:00 +0000\r\n", ""))
			}
			captureThreadMail(t, e, e.Rep1, raw)
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
			dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				var source ids.UUID
				if err := tx.QueryRow(ctx, `SELECT activity_id FROM activity_identity WHERE identity_kind = 'mail' AND identity_key = $1`, "retained-date@buyer.test").Scan(&source); err != nil {
					return err
				}
				if err := capture.NewPendingStore(e.DB()).PurgeRawCaptureTx(ctx, tx, []ids.UUID{source}); err != nil {
					return err
				}
				c := Commitment{
					Meaning: openCommitmentMeaning("", ""), AsOf: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), SourceActivityID: source, ReadMessages: []ids.UUID{source}, Extractor: signalScanActor, Locator: commitmentLocator(source, claimKindOurs, e.Rep1.String(), "Send pricing"), Seat: &e.Rep1, Summary: "Send pricing", Confidence: 0.95,
				}
				outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
				want := CommitmentOutcome(activities.CommitmentSourceUnknown)
				if dated {
					want = CommitmentTaskWritten
				}
				if err == nil && outcome != want {
					t.Errorf("outcome = %q, want %q", outcome, want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
