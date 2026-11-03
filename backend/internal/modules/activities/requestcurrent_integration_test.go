// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestCurrentRequestReadAdmitsBeforeLimitingAndRechecksSourceAccess(t *testing.T) {
	e := setupLoad(t)
	query := ids.NewV7().String()
	want := seedEmailRequest(t, e, query+" Please send the report", "commitment", OwedVerdictAsksUs)
	seedEmailRequest(t, e, query+" Thanks for the update", "noise", OwedVerdictInformsUs)
	store := storeKnowing(e)
	at := requestInstant.Add(time.Hour)
	rows, _, err := store.ListActivities(e.as(), ListActivitiesInput{RequestCurrentAsOf: &at, Query: &query, Limit: new(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || ids.UUID(rows[0].Id) != want {
		t.Fatalf("current request missing from bounded read: %+v", rows)
	}
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	writer := principal.WithActor(e.as(), actor)
	direction, subject := "inbound", query+" Please confirm the scope"
	private, _, err := store.LogActivity(writer, LogActivityInput{
		Kind: "email", Direction: &direction, Subject: &subject,
		OccurredAt: &requestInstant, ThreadKey: "hand-recorded-request", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	privateID := ids.UUID(private.Id)
	if _, _, err := store.LogActivity(writer, LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &privateID}); err != nil {
		t.Fatal(err)
	}
	rows, _, err = store.ListActivities(writer, ListActivitiesInput{RequestCurrentAsOf: &at, Query: &query})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		found = found || row.Id == private.Id
	}
	if !found {
		t.Fatal("human-held request missing before its source was narrowed")
	}
	if _, err := store.SetAudience(writer, ids.From[ids.ActivityKind](privateID), SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	reader := principal.WithActor(e.as(), actor)
	rows, _, err = store.ListActivities(reader, ListActivitiesInput{RequestCurrentAsOf: &at, Query: &query})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || ids.UUID(rows[0].Id) != want {
		t.Fatalf("private request affected the reader's current work: %+v", rows)
	}
}

func TestCurrentRequestsKeepEachReadersDispositionAndReplyWake(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send prices", "commitment", OwedVerdictAsksUs)
	at := requestInstant.Add(time.Hour)
	store := storeKnowing(e).WithClock(func() time.Time { return at })
	reader := e.asSeat(e.rep)
	request := ids.From[ids.ActivityKind](id)
	assertCurrent := func(want bool) {
		t.Helper()
		rows, _, err := store.ListActivities(reader, ListActivitiesInput{RequestCurrentAsOf: &at})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			found = found || ids.UUID(row.Id) == id
		}
		if found != want {
			t.Fatalf("current request = %v, want %v", found, want)
		}
	}
	assertCurrent(true)
	for _, judge := range []func() error{
		func() error { return store.SetMessageNotMine(reader, request) },
		func() error { return store.MarkMessageHandled(reader, request) },
		func() error {
			return store.SnoozeMessage(reader, request, values.ReopenOnTime, new(at.Add(time.Hour)), nil)
		},
	} {
		if err := judge(); err != nil {
			t.Fatal(err)
		}
		assertCurrent(false)
		if err := store.ClearMessageDisposition(reader, request); err != nil {
			t.Fatal(err)
		}
		assertCurrent(true)
	}
	if err := store.SnoozeMessage(reader, request, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatal(err)
	}
	assertCurrent(false)
	source, err := store.GetActivity(reader, request, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Minute)
	direction, subject := "inbound", "Re: Please send prices"
	if _, _, err := store.LogActivity(reader, LogActivityInput{
		Kind: "email", Source: "manual", Direction: &direction, Subject: &subject,
		ThreadKey: *source.ThreadKey, CounterpartyEmail: requestCounterparty, OccurredAt: &at,
	}); err != nil {
		t.Fatal(err)
	}
	assertCurrent(true)
}
