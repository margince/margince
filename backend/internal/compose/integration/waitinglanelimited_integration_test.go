// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A waiting mail limited to its participants stays off a colleague's waiting
// list, while its open twin on the same contact reaches them. Rep3 is
// addressed by both, so the limited one's absence is the audience's doing.
func TestALimitedWaitingMailStaysOffAColleaguesWaitingList(t *testing.T) {
	e := Setup(t)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	mail := func(thread, audience string) ids.UUID {
		id := seedWaitingMessageLinked(t, e, thread, "inbound", "Re: renewal", waitingInstant.Add(-3*24*time.Hour), dana)
		if _, err := OwnerConn(t).Exec(context.Background(), `
			WITH limited AS (UPDATE activity SET audience = $2 WHERE id = $1)
			INSERT INTO activity_participant (activity_id, role, address, user_id)
			SELECT $1, 'to', lower(email), id FROM app_user WHERE id = $3`, id, audience, e.Rep3); err != nil {
			t.Fatalf("addressing the %s mail to Rep3: %v", audience, err)
		}
		return id
	}
	limited, open := mail("thread-limited", "participants"), mail("thread-open", "workspace")
	asOf := waitingInstant
	waiting := func(ctx context.Context) map[ids.UUID]bool {
		got, _, err := e.Activities.ListActivities(ctx, activities.ListActivitiesInput{WaitingReplyAsOf: &asOf})
		if err != nil {
			t.Fatalf("listing waiting replies: %v", err)
		}
		listed := map[ids.UUID]bool{}
		for _, a := range got {
			listed[ids.UUID(a.Id)] = true
		}
		return listed
	}
	holder := waiting(e.As(e.Rep3, []ids.UUID{e.Team2}, activityLinkRepPerms))
	if !holder[limited] || !holder[open] {
		t.Fatalf("Rep3, addressed by both, lists limited=%t open=%t; want both", holder[limited], holder[open])
	}
	colleague := waiting(e.As(e.Rep1, []ids.UUID{e.Team1}, activityLinkRepPerms))
	if !colleague[open] {
		t.Fatal("Rep1 does not list the open mail, so the limited one's absence proves nothing")
	}
	if colleague[limited] {
		t.Fatal("Rep1 lists a waiting mail limited to participants they are not among")
	}
}
