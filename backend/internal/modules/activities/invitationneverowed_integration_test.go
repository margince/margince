// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// invitationAsking is an invitation the classifier judged asks_us, the verdict
// a calendar's "Reply for" line and an agenda draw.
func invitationAsking(t *testing.T, e *loadEnv) ids.UUID {
	t.Helper()
	return seedCalendarMail(t, e, "REQUEST", OwedVerdictAsksUs)
}

// meetingWithUID gives the invitation a calendar UID and the meeting synced
// from it, starting at startsAt.
func meetingWithUID(t *testing.T, e *loadEnv, invitation ids.UUID, startsAt time.Time) {
	t.Helper()
	uid := "kickoff-" + ids.NewV7().String() + "@google.com"
	meeting := ids.NewV7()
	e.exec(t, `UPDATE activity SET calendar_uid = $2 WHERE id = $1`, invitation, uid)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kickoff', $2, 'seed', 'system')`, meeting, startsAt)
	e.exec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'seed')`, MeetingIdentityKey(uid, startsAt.Format(time.RFC3339)), meeting)
}

// offeredOnTheRecord reports whether the deal card's two request reads name
// the message: the current request and the request offered for review.
func offeredOnTheRecord(t *testing.T, e *loadEnv, id ids.UUID, ownDomains ...string) (current, review bool) {
	t.Helper()
	at := requestInstant.Add(time.Hour)
	for _, in := range []struct {
		input ListActivitiesInput
		found *bool
	}{
		{ListActivitiesInput{RequestCurrentAsOf: &at}, &current},
		{ListActivitiesInput{RequestReviewAsOf: &at}, &review},
	} {
		rows, _, err := storeKnowing(e, ownDomains...).ListActivities(e.as(), in.input)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			*in.found = *in.found || ids.UUID(row.Id) == id
		}
	}
	return current, review
}

func TestAColleaguesInvitationIsNeverOwedAReply(t *testing.T) {
	e := setupLoad(t)
	invitation := invitationAsking(t, e)
	e.exec(t, `UPDATE activity SET invitation_sender = $2 WHERE id = $1`, invitation, e.other)
	if owedNow(t, e, invitation) {
		t.Error("a colleague's invitation the classifier judged asks_us is owed a reply")
	}
	if current, review := offeredOnTheRecord(t, e, invitation); current || review {
		t.Errorf("the record offers a colleague's invitation as a request: current %v, review %v", current, review)
	}
}

func TestAnInvitationForAMeetingAlreadyHeldIsNeverOwedAReply(t *testing.T) {
	e := setupLoad(t)
	held := invitationAsking(t, e)
	meetingWithUID(t, e, held, requestInstant.Add(-24*time.Hour))
	ahead := invitationAsking(t, e)
	meetingWithUID(t, e, ahead, requestInstant.Add(48*time.Hour))
	if owedNow(t, e, held) {
		t.Error("an invitation for a meeting already held is owed a reply")
	}
	if current, review := offeredOnTheRecord(t, e, held); current || review {
		t.Errorf("the record offers an invitation for a held meeting: current %v, review %v", current, review)
	}
	// An outside invitation for a meeting ahead keeps its asks_us verdict. Its
	// badge offers the RSVP, so the card's request read is where it shows.
	if current, _ := offeredOnTheRecord(t, e, ahead); !current {
		t.Error("an outside invitation judged asks_us for a meeting ahead is no longer a request, so the case above proves nothing")
	}
}

// A reminder the request pass filed before the invitation was known to be a
// colleague's is retired by the next pass.
func TestTheRequestPassRetiresAReminderForAColleaguesInvitation(t *testing.T) {
	e := setupLoad(t)
	invitation := invitationAsking(t, e)
	pass := func() {
		t.Helper()
		if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	pass()
	openReminders := func() int {
		t.Helper()
		var n int
		if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request'
			 AND source_activity_id = $1 AND archived_at IS NULL`, invitation).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if openReminders() != 1 {
		t.Fatal("the pass filed no reminder for an outside invitation judged asks_us, so the retire below proves nothing")
	}
	e.exec(t, `UPDATE activity SET invitation_sender = $2 WHERE id = $1`, invitation, e.other)
	pass()
	if n := openReminders(); n != 0 {
		t.Errorf("%d reminders stay open for a colleague's invitation, want 0", n)
	}
}

// The record's request reads apply the Worklist's colleague test: mail from one
// of our own domains is not a customer waiting on the deal.
func TestTheRecordOffersNoColleaguesMailAsARequest(t *testing.T) {
	e := setupLoad(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}}
	colleague := seedEmailRequestFiledUnder(t, e, "Can you send the report?", "commitment", OwedVerdictAsksUs,
		"colleague@ours.test", links, mailParticipants{From: "colleague@ours.test"})
	if current, _ := offeredOnTheRecord(t, e, colleague); !current {
		t.Fatal("the request is not offered even with no own domain known, so the case below proves nothing")
	}
	if current, review := offeredOnTheRecord(t, e, colleague, "ours.test"); current || review {
		t.Errorf("the record offers a colleague's mail as a request: current %v, review %v", current, review)
	}
	// A rep who takes it on as a request keeps it on the record.
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	if _, _, err := storeKnowing(e).LogActivity(principal.WithActor(e.as(), actor), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &colleague}); err != nil {
		t.Fatal(err)
	}
	if current, _ := offeredOnTheRecord(t, e, colleague, "ours.test"); !current {
		t.Error("a colleague's mail a rep took on as a request left the record")
	}
}

// A reminder for an invitation whose meeting has begun is retired too. So is
// one for an invitation from our own domain that carries no seat stamp.
func TestTheRequestPassRetiresRemindersForHeldAndOwnDomainInvitations(t *testing.T) {
	e := setupLoad(t)
	held := invitationAsking(t, e)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}}
	ownDomain := seedEmailRequestFiledUnder(t, e, "Invitation: Sync", "meeting", OwedVerdictAsksUs,
		"colleague@ours.test", links, mailParticipants{From: "colleague@ours.test"})
	setCalendarMethod(t, e, ownDomain, "REQUEST")
	at := requestInstant.Add(time.Hour)
	if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), at); err != nil {
		t.Fatal(err)
	}
	meetingWithUID(t, e, held, requestInstant.Add(-24*time.Hour))
	if err := storeKnowing(e, "ours.test").CaptureEmailRequests(asClassifier(e), at); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]ids.UUID{"held meeting": held, "own domain": ownDomain} {
		var open int
		if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request'
			 AND source_activity_id = $1 AND archived_at IS NULL`, id).Scan(&open); err != nil {
			t.Fatal(err)
		}
		if open != 0 {
			t.Errorf("%s: %d reminders stay open, want 0", name, open)
		}
	}
}

// The settlement pass judges whether our reply answered a request. An
// invitation for a meeting already held asked for an RSVP, never for words, so
// there is nothing to judge.
func TestTheSettlementPassJudgesNoInvitationThatIsNeverOwed(t *testing.T) {
	e := setupLoad(t)
	held := invitationAsking(t, e)
	meetingWithUID(t, e, held, requestInstant.Add(-24*time.Hour))
	ahead := invitationAsking(t, e)
	meetingWithUID(t, e, ahead, requestInstant.Add(72*time.Hour))
	for _, invitation := range []ids.UUID{held, ahead} {
		replyTo(t, e, invitation, "Thanks, see you there.", requestInstant.Add(time.Hour))
	}

	candidates := repliedIDs(t, e, requestInstant.Add(2*time.Hour))
	if _, offered := candidates[held]; offered {
		t.Error("the settlement pass offers an invitation for a meeting already held")
	}
	if _, offered := candidates[ahead]; !offered {
		t.Error("an outside invitation for a meeting ahead is no longer offered, so the case above proves nothing")
	}
}

// The response rate counts replies to customers waiting on a written answer.
// An answer to an invitation that is never owed one is not such a reply.
func TestTheResponseRateCountsNoAnswerToANeverOwedInvitation(t *testing.T) {
	e := setupLoad(t)
	from, to := requestInstant.Add(-24*time.Hour), requestInstant.Add(24*time.Hour)
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	held := invitationAsking(t, e)
	meetingWithUID(t, e, held, requestInstant.Add(-24*time.Hour))
	replyTo(t, e, held, "Thanks, see you there.", requestInstant.Add(time.Hour))

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if after.Answered != before.Answered {
		t.Errorf("an answer to an invitation for a held meeting moved the answered count %d → %d", before.Answered, after.Answered)
	}
	request := seedEmailRequest(t, e, "Angebot", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Anbei das Angebot.", requestInstant.Add(time.Hour))
	control, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if control.Answered != after.Answered+1 {
		t.Errorf("an answered request moved the count %d → %d, so the case above proves nothing", after.Answered, control.Answered)
	}
}

// A company finding that cites an invitation for a held meeting says no question
// is waiting: SettledCitations reads it as settled, as the Worklist does.
func TestACitedInvitationForAHeldMeetingIsSettled(t *testing.T) {
	e := setupLoad(t)
	held := invitationAsking(t, e)
	meetingWithUID(t, e, held, requestInstant.Add(-24*time.Hour))
	ahead := invitationAsking(t, e)
	meetingWithUID(t, e, ahead, requestInstant.Add(72*time.Hour))

	var settled map[ids.UUID]bool
	reader := e.asSeat(e.rep)
	if err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(reader, func(tx pgx.Tx) error {
		var err error
		settled, err = SettledCitations(reader, tx, []ids.UUID{held, ahead}, nil, requestInstant.Add(time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !settled[held] {
		t.Error("an invitation for a held meeting still reads as a question waiting for an answer")
	}
	if settled[ahead] {
		t.Error("an outside invitation for a meeting ahead reads as settled, so the case above proves nothing")
	}
}
