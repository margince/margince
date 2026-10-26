// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A rep promised a partner "this week" on a Monday; the partner only said
// thanks. Eleven days later the promise is the rep's task, due that Friday,
// and overdue on the contact.
func TestAPromiseForThisWeekIsDueThatFridayAndAnAcknowledgementDoesNotKeepIt(t *testing.T) {
	for name, week := range map[string]struct {
		workdays []int
		away     bool
		lastDay  int
	}{
		"a Monday-to-Friday rep":                      {nil, false, 16},
		"a rep who works Monday to Thursday":          {[]int{1, 2, 3, 4}, false, 15},
		"a rep in another zone than the installation": {[]int{1, 2, 3, 4, 5}, true, 16},
	} {
		t.Run(name, func(t *testing.T) { promiseForThisWeek(t, week.workdays, week.away, week.lastDay) })
	}
}

// awayZone is a zone other than the installation's, so a deadline read in the
// installation's zone would be hours off.
func awayZone(t *testing.T, installation *time.Location) *time.Location {
	t.Helper()
	name := "Asia/Ho_Chi_Minh"
	if installation.String() == name {
		name = "America/New_York"
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

func promiseForThisWeek(t *testing.T, workdays []int, away bool, lastDay int) {
	e := integration.Setup(t)
	zone := recordZoneOf(t, e)
	if away {
		zone = awayZone(t, zone)
	}
	if workdays != nil {
		rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
		if _, err := identity.NewService(e.Pool).SaveMyWorkingHours(rep, identity.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: workdays, Timezone: zone.String()}); err != nil {
			t.Fatalf("saving the rep's working week: %v", err)
		}
	}
	said := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	asOf := said.AddDate(0, 0, 11)
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Pia Partner", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "pia@partner.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(row.Id)
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	promise, thanks := "The contract is in final legal review. I will send it to you this week to sign.", "OK, thanks."
	out, in, ackAt := "outbound", "inbound", said.AddDate(0, 0, 2)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &promise, Direction: &out, OccurredAt: &said, Source: "manual", ThreadKey: "thread-contract",
		CounterpartyEmail: "pia@partner.example", CounterpartyOutboundAttested: true, Links: links,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &thanks, Direction: &in, OccurredAt: &ackAt, Source: "manual", ThreadKey: "thread-contract",
		CounterpartyEmail: "pia@partner.example", Links: links,
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	c := Commitment{
		AsOf: asOf, ReadMessages: []ids.UUID{sourceID, ids.UUID(ack.Id)}, Extractor: signalScanActor,
		SourceActivityID: sourceID, Summary: "Send the contract to sign", Quote: "I will send it to you this week to sign.", Body: promise,
		Seat: &e.Rep1, Confidence: 1, Links: links,
		Meaning: &commitmentMeaning{State: commitmentStateOpen, DeadlinePrecision: commitmentPrecisionRange, DeadlineQuote: "this week"},
		Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), "I will send it to you this week to sign."),
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		got, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		if err == nil && got != CommitmentTaskWritten {
			t.Errorf("dispatch = %s, want the rep's task", got)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var due *time.Time
	var assignee ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT due_at, assignee_id FROM activity WHERE kind = 'task' AND source_activity_id = $1`, sourceID).Scan(&due, &assignee)
	}); err != nil {
		t.Fatalf("reading the promise's task: %v", err)
	}
	if want := time.Date(2026, 10, lastDay, 23, 59, 59, 0, zone); due == nil || !due.Equal(want) || assignee != e.Rep1 {
		t.Fatalf("task due %v for %s, want %v for the rep", due, assignee, want)
	}
	moment := momentOf(e.Admin(), t, pageAsOf(e, asOf), ids.From[ids.ContactKind](contact))
	if moment.Rule != crmcontracts.ContactMomentRuleOverduePromise {
		t.Fatalf("the late promise is not overdue on the contact: %s %q", moment.Rule, moment.Headline)
	}
}

func recordZoneOf(t *testing.T, e *integration.Env) *time.Location {
	t.Helper()
	var zone *time.Location
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		zone, err = installationZoneTx(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return zone
}

// A promise renewed "this week" in a later message is dated in the renewal's
// week. The renewal is the message that states it.
func TestARenewedPromiseForThisWeekIsDatedInTheRenewalsWeek(t *testing.T) {
	e := integration.Setup(t)
	original := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	renewedAt := original.AddDate(0, 0, 7)
	out := "outbound"
	first, second := "I will send the contract this week.", "Sorry for the delay, I will send it this week."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Body: &first, Direction: &out, OccurredAt: &original, Source: "manual", ThreadKey: "thread-renewal", CounterpartyOutboundAttested: true})
	if err != nil {
		t.Fatal(err)
	}
	renewal, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Body: &second, Direction: &out, OccurredAt: &renewedAt, Source: "manual", ThreadKey: "thread-renewal", CounterpartyOutboundAttested: true})
	if err != nil {
		t.Fatal(err)
	}
	renewalID := ids.UUID(renewal.Id)
	c := Commitment{
		AsOf: renewedAt.AddDate(0, 0, 1), ReadMessages: []ids.UUID{ids.UUID(source.Id), renewalID},
		SourceActivityID: ids.UUID(source.Id), AdmissionSourceID: &renewalID, RenewedAt: &renewedAt,
		Meaning: &commitmentMeaning{State: commitmentStateOpen, DeadlinePrecision: commitmentPrecisionRange, DeadlineQuote: "this week"},
		Seat:    &e.Rep1,
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	var dated Commitment
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		dated, err = withStatedWeek(ctx, tx, c)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if dated.DueDate != "2026-10-16" {
		t.Fatalf("the renewed promise is due %q, want the Friday of the renewal's week", dated.DueDate)
	}
}
