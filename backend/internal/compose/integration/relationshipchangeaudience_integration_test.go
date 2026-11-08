// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The derived relationship CHANGE ("replied after N quiet days") is one
// reader's follow-up advice, so it counts the messages that reader can open.
// A held message moves it for a reader in its audience and for nobody else.
// The shared strength score stays workspace-only (contacts/strength.go).

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// TestALimitedReplyIsNotDerivedIntoARelationshipChange seeds the shape
// ChangeRepliedAfterGap fires on: an old outbound, then a recent reply. Open,
// the change fires for a colleague. Held, it fires for the author, who can open
// the reply, and not for the colleague, who cannot.
func TestALimitedReplyIsNotDerivedIntoARelationshipChange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		audience string
		reader   string
		want     bool
	}{
		{"open to the workspace, the change fires for a colleague", "workspace", "colleague", true},
		{"held, the author who can open it sees the change", "participants", "author", true},
		{"held, a colleague outside its audience does not", "participants", "colleague", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
			readers := map[string]context.Context{"author": author, "colleague": e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms)}
			contact := e.SeedContact(t, "Ines Wieder", &e.Rep1)

			now := time.Now().UTC()
			link := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}

			// The far side of the silence: an outbound long before the window.
			// Always workspace-visible, so the only variable is the reply.
			openSubject := "Angebot nachgefasst"
			oldTouch, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
				Kind: "email", Subject: &openSubject, Direction: new("outbound"), Links: link,
			})
			if err != nil {
				t.Fatalf("logging the outbound that opens the silence: %v", err)
			}
			backdate(t, ids.UUID(oldTouch.Id), now.AddDate(0, 0, -120))

			replySubject, replyBody := "Re: Angebot", "intern noch nicht spruchreif"
			reply, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
				Kind: "email", Subject: &replySubject, Body: &replyBody,
				Direction: new("inbound"), Links: link,
			})
			if err != nil {
				t.Fatalf("logging the reply: %v", err)
			}
			backdate(t, ids.UUID(reply.Id), now.AddDate(0, 0, -1))
			// Through the real writer: SetAudience is what a human uses.
			if _, err := e.Activities.SetAudience(author,
				ids.From[ids.ActivityKind](ids.UUID(reply.Id)),
				activities.SetAudienceInput{Audience: tc.audience}); err != nil {
				t.Fatalf("setting the reply audience to %s: %v", tc.audience, err)
			}

			got := hasKind(changesFor(readers[tc.reader], t, e, contact, now), relstrength.ChangeRepliedAfterGap)
			if got != tc.want {
				t.Errorf("a %s reply read by the %s produced %s = %v, want %v",
					tc.audience, tc.reader, relstrength.ChangeRepliedAfterGap, got, tc.want)
			}
		})
	}
}

// changesFor reads one contact's derived changes as one caller.
func changesFor(ctx context.Context, t *testing.T, e *Env, contact ids.UUID, now time.Time) []relstrength.Change {
	t.Helper()
	return changesOf(ctx, t, e, ids.From[ids.ContactKind](contact), now)
}

// backdate moves a logged activity's occurred_at, which LogActivity stamps at
// now. The gap the change is about is measured in months, and no test may wait
// for one.
func backdate(t *testing.T, activity ids.UUID, at time.Time) {
	t.Helper()
	owner := OwnerConn(t)
	if _, err := owner.Exec(t.Context(),
		`UPDATE activity SET occurred_at = $2 WHERE id = $1`, activity, at); err != nil {
		t.Fatalf("backdating an activity: %v", err)
	}
}

func hasKind(changes []relstrength.Change, kind string) bool {
	for _, c := range changes {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

// TestAHeldMessageDoesNotShortenTheGapAReplyBroke covers the SECOND audience
// clause, which the test above cannot reach: it holds the reply, so
// LatestInbound is nil and changeInputs returns before the preceding-interaction
// query ever runs.
//
// The gap is measured back to the interaction before the reply. A held message
// in that span shortens the silence only for a reader who can open it. Twelve
// days is below ReplyGapDays, so counting it takes the change away entirely.
func TestAHeldMessageDoesNotShortenTheGapAReplyBroke(t *testing.T) {
	for _, tc := range []struct {
		name        string
		intervening string
		reader      string
		wantFires   bool
	}{
		{"an open message in the span shortens the gap below the threshold", "workspace", "colleague", false},
		{"a held one shortens it for the author, who can open it", "participants", "author", false},
		{"a held one leaves the silence for a colleague outside its audience", "participants", "colleague", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
			readers := map[string]context.Context{"author": author, "colleague": e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms)}
			contact := e.SeedContact(t, "Wenke Dazwischen", &e.Rep1)

			now := time.Now().UTC()
			link := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}

			log := func(subject, direction string, daysAgo int, audience string) {
				t.Helper()
				logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
					Kind: "email", Subject: &subject, Direction: new(direction), Links: link,
				})
				if err != nil {
					t.Fatalf("logging %q: %v", subject, err)
				}
				backdate(t, ids.UUID(logged.Id), now.AddDate(0, 0, -daysAgo))
				if audience != "workspace" {
					if _, err := e.Activities.SetAudience(author,
						ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
						activities.SetAudienceInput{Audience: audience}); err != nil {
						t.Fatalf("setting %q to %s: %v", subject, audience, err)
					}
				}
			}

			// The far side of the silence, the message under test inside it,
			// then the reply that broke it. Only the middle one varies.
			log("Angebot nachgefasst", "outbound", 121, "workspace")
			log("Zwischenstand", "outbound", 13, tc.intervening)
			log("Re: Angebot", "inbound", 1, "workspace")

			changes := changesFor(readers[tc.reader], t, e, contact, now)
			var got *relstrength.Change
			for i := range changes {
				if changes[i].Kind == relstrength.ChangeRepliedAfterGap {
					got = &changes[i]
				}
			}
			if tc.wantFires {
				if got == nil {
					t.Fatalf("no %s in %+v — a held message moved the gap it must not move",
						relstrength.ChangeRepliedAfterGap, changes)
				}
				// 120, not 12: measured past the held message to the open one
				// before it.
				if got.Days != 120 {
					t.Errorf("the reply is reported as breaking a %d-day silence, want 120 — "+
						"a held message in the span shortened it", got.Days)
				}
				return
			}
			if got != nil {
				t.Errorf("%s fired after %d days although the %s can open a message 12 days ago",
					got.Kind, got.Days, tc.reader)
			}
		})
	}
}
