// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The fifth reader of the audience column: the derived relationship CHANGE.
//
// auth.AudienceWorkspaceOnly's own doc names four aggregate readers and states
// the rule they exist for — a last-activity timestamp that moves when a private
// message arrives tells a colleague when it arrived, without showing a word of
// it. relationship strength is one of the four and applies the clause in five
// places (contacts/strength.go). The change DERIVED from that strength applied it
// in none, so a message the score itself refused to count still produced
// "replied after 41 quiet days" with the reply's own timestamp.
//
// That is the disclosure the rule forbids, arriving through arithmetic rather
// than through a row. It is asserted here rather than in the module because the
// leak is only visible end to end: the fold is pure, the SQL is what withholds,
// and only a real limited message proves which of the two is answering.
//
// The control is the SAME message at workspace audience, not the same message
// read by its author. AudienceWorkspaceOnly is deliberately not the reader's own
// audience — "the question an aggregate asks is may EVERYONE see this, and the
// answer is the same for everybody" — so a fixture asserting the author still
// sees the change would be asserting the opposite of the rule, and would have to
// fail for the change to be right.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// TestALimitedReplyIsNotDerivedIntoARelationshipChange seeds the exact shape
// ChangeRepliedAfterGap fires on — one old outbound, then a recent inbound reply
// after a long silence — and runs it twice over the same fixture: once with the
// reply open to the workspace, once held to its participants. Open, the change
// fires. Held, it does not, for anybody.
//
// Two arms over one fixture rather than two readers over one message, because
// the aggregate rule is about the MESSAGE and not about who is asking. The open
// arm is the positive control: without it, "no change" is also what a fixture
// that fires for nobody produces, and the held arm would pass against a broken
// seed.
func TestALimitedReplyIsNotDerivedIntoARelationshipChange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		audience string
		want     bool
	}{
		{"open to the workspace, the change fires", "workspace", true},
		{"held to its participants, it does not", "participants", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
			contact := e.SeedContact(t, "Ines Wieder", &e.Rep1)

			now := time.Now().UTC()
			link := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}

			// The far side of the silence: an outbound long before the window.
			// Always workspace-visible, so the only variable is the reply.
			openSubject := "Angebot nachgefasst"
			oldTouch, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
				Kind: "email", Subject: &openSubject, Direction: strPtr("outbound"), Links: link,
			})
			if err != nil {
				t.Fatalf("logging the outbound that opens the silence: %v", err)
			}
			backdate(t, ids.UUID(oldTouch.Id), now.AddDate(0, 0, -120))

			replySubject, replyBody := "Re: Angebot", "intern noch nicht spruchreif"
			reply, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
				Kind: "email", Subject: &replySubject, Body: &replyBody,
				Direction: strPtr("inbound"), Links: link,
			})
			if err != nil {
				t.Fatalf("logging the reply: %v", err)
			}
			backdate(t, ids.UUID(reply.Id), now.AddDate(0, 0, -1))
			// Through the real writer: SetAudience is what a human uses, and a
			// hand-written UPDATE would prove nothing about the path that runs.
			if _, err := e.Activities.SetAudience(author,
				ids.From[ids.ActivityKind](ids.UUID(reply.Id)),
				activities.SetAudienceInput{Audience: tc.audience}); err != nil {
				t.Fatalf("setting the reply audience to %s: %v", tc.audience, err)
			}

			got := hasKind(changesFor(author, t, e, contact, now), relstrength.ChangeRepliedAfterGap)
			if got != tc.want {
				t.Errorf("a %s reply produced %s = %v, want %v — a change derived from a "+
					"message discloses that message's timing and the silence it broke",
					tc.audience, relstrength.ChangeRepliedAfterGap, got, tc.want)
			}
		})
	}
}

// changesFor reads one contact's derived changes as one caller.
func changesFor(ctx context.Context, t *testing.T, e *Env, contact ids.UUID, now time.Time) []relstrength.Change {
	t.Helper()
	var out []relstrength.Change
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = e.Contacts.ContactRelationshipChangesTx(ctx, tx,
			ids.From[ids.ContactKind](contact), now, nil)
		return err
	}); err != nil {
		t.Fatalf("reading relationship changes: %v", err)
	}
	return out
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
// The gap is measured back to the interaction BEFORE the reply. A held message
// sitting inside that span is still an interaction, so counting it would shorten
// the silence — and a silence that shortens is itself the disclosure: the number
// changes on a day the reader is not allowed to see, and the change says
// something happened then.
//
// Both arms use the SAME open reply and differ only in the audience of one
// message in the middle. Twelve days is below ReplyGapDays, so counting it does
// not merely change the number — it takes the change away entirely, which is a
// sharper control than a number nobody would notice was wrong.
func TestAHeldMessageDoesNotShortenTheGapAReplyBroke(t *testing.T) {
	for _, tc := range []struct {
		name        string
		intervening string
		wantFires   bool
	}{
		{"an open message in the span shortens the gap below the threshold", "workspace", false},
		{"a held one leaves the silence as the workspace sees it", "participants", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
			contact := e.SeedContact(t, "Wenke Dazwischen", &e.Rep1)

			now := time.Now().UTC()
			link := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}

			log := func(subject, direction string, daysAgo int, audience string) {
				t.Helper()
				logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
					Kind: "email", Subject: &subject, Direction: strPtr(direction), Links: link,
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

			changes := changesFor(author, t, e, contact, now)
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
				t.Errorf("%s fired after %d days with an OPEN message 12 days ago — the "+
					"control arm is supposed to fall below ReplyGapDays, so this fixture "+
					"is not proving the held arm means anything", got.Kind, got.Days)
			}
		})
	}
}
