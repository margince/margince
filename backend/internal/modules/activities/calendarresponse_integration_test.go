// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedCalendarMail is a contact's mail carrying a calendar part with method,
// labelled `meeting` as the capture classifier labels one. An empty verdict
// leaves the message unjudged.
func seedCalendarMail(t *testing.T, e *loadEnv, method, verdict string) ids.UUID {
	t.Helper()
	return seedCalendarMailSaying(t, e, method, verdict, "Buyer has declined this invitation.")
}

func seedCalendarMailSaying(t *testing.T, e *loadEnv, method, verdict, text string) ids.UUID {
	t.Helper()
	store := storeKnowing(e)
	direction, subject := "inbound", "Declined: Demo @ Thu 15:00"
	body := "From: " + requestCounterparty + "\n\n" + text
	source, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: ids.NewV7().String(), Source: "test", OccurredAt: &requestInstant,
		CounterpartyEmail: requestCounterparty, EmailTo: []string{seatAddress(e.rep)},
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, e.rep)
	if _, err := store.SetCaptureLabel(asClassifier(e), id, "meeting"); err != nil {
		t.Fatal(err)
	}
	if method != "" {
		setCalendarMethod(t, e, id, method)
	}
	if verdict != "" {
		if _, err := store.SetOwedVerdict(asClassifier(e), id, verdict, "prompts-test", dbNow(t, e)); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// setCalendarMethod writes the method as the re-read of a stored original does.
func setCalendarMethod(t *testing.T, e *loadEnv, id ids.UUID, method string) {
	t.Helper()
	if err := database.WithWorkspaceTx(asClassifier(e), e.pool, func(tx pgx.Tx) error {
		_, err := SetCalendarMethodTx(asClassifier(e), tx, ids.From[ids.ActivityKind](id), method)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// owedNow reports whether the message is on the waiting lane, failing when its
// badge disagrees. Both read owedSQL; nothing here takes the mail on as a task.
func owedNow(t *testing.T, e *loadEnv, id ids.UUID) bool {
	t.Helper()
	waiting, err := storeKnowing(e).WaitingReplies(e.as(), requestInstant.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	onLane := false
	for _, row := range waiting {
		onLane = onLane || row.ActivityID == id
	}
	if needsReply := badgeNeedsReply(t, e, id); onLane != needsReply {
		t.Fatalf("the waiting lane says %v and the badge says %v about one message", onLane, needsReply)
	}
	return onLane
}

func badgeNeedsReply(t *testing.T, e *loadEnv, id ids.UUID) bool {
	t.Helper()
	summaries, err := storeKnowing(e).EmailSummariesByID(e.as(), []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	return summaries[id].Move == crmcontracts.EmailSummaryMoveNeedsReply
}

func TestADeclinedInvitationIsOwedNothingUnlessItAsks(t *testing.T) {
	e := setupLoad(t)
	if !owedNow(t, e, seedCalendarMail(t, e, "REPLY", OwedVerdictAsksUs)) {
		t.Error("a calendar reply the classifier says asks us is not owed")
	}
	if owedNow(t, e, seedCalendarMail(t, e, "REPLY", "")) {
		t.Error("an unjudged calendar reply is owed through its meeting label or as unanswered mail")
	}
	if owedNow(t, e, seedCalendarMail(t, e, "DECLINECOUNTER", OwedVerdictInformsUs)) {
		t.Error("a calendar reply the classifier says informs us is owed")
	}
	// An invitation asks for an RSVP, never for a written reply, unless judged.
	for _, method := range []string{"REQUEST", "CANCEL", "PUBLISH"} {
		if owedNow(t, e, seedCalendarMail(t, e, method, "")) {
			t.Errorf("an unjudged %s from a contact is owed a reply", method)
		}
	}
	if !owedNow(t, e, seedCalendarMail(t, e, "REQUEST", OwedVerdictAsksUs)) {
		t.Error("an invitation the classifier says asks us is not owed")
	}
	if !owedNow(t, e, seedCalendarMail(t, e, "", "")) {
		t.Error("plain unjudged mail from a contact is no longer owed, so the cases above prove nothing")
	}
}

// A rep took the mail on before its method was read back from the original.
// The lane hands an accepted request to its reminder task, so the badge is read.
func TestACalendarReplyARepTookOnStaysOwed(t *testing.T) {
	e := setupLoad(t)
	id := seedCalendarMail(t, e, "", "")
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	reader := principal.WithActor(e.as(), actor)
	if _, _, err := storeKnowing(e).LogActivity(reader, LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &id}); err != nil {
		t.Fatal(err)
	}
	setCalendarMethod(t, e, id, "REPLY")
	if !badgeNeedsReply(t, e, id) {
		t.Error("a calendar reply a rep accepted as a request is not owed")
	}
}

// The record offers unjudged mail as a request to take on; a calendar copy is
// not one.
func TestTheRecordOffersNoUnjudgedCalendarReplyForReview(t *testing.T) {
	e := setupLoad(t)
	decline := seedCalendarMail(t, e, "REPLY", "")
	invitation := seedCalendarMail(t, e, "REQUEST", "")
	plain := seedCalendarMail(t, e, "", "")
	asOf := requestInstant.Add(time.Hour)
	review, _, err := storeKnowing(e).ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &asOf})
	if err != nil {
		t.Fatal(err)
	}
	offered := map[ids.UUID]bool{}
	for _, row := range review {
		offered[ids.UUID(row.Id)] = true
	}
	if offered[decline] {
		t.Error("an unjudged calendar reply is offered as a request to take on")
	}
	if offered[invitation] {
		t.Error("an unjudged invitation is offered as a request to take on")
	}
	if !offered[plain] {
		t.Error("unjudged plain mail is no longer offered for review, so the cases above prove nothing")
	}
}

// A sender can mark any request REPLY. An unjudged calendar response stays off
// the lane, but the classifier is still offered it, and its verdict decides.
func TestAForgedCalendarReplyIsStillJudged(t *testing.T) {
	e := setupLoad(t)
	forged := seedCalendarMailSaying(t, e, "REPLY", "",
		"Please send us a quote for 40 seats by Friday.")
	if owedNow(t, e, forged) {
		t.Fatal("an unjudged calendar reply is on the lane before it was judged")
	}
	backlog, _, err := storeKnowing(e).OwedBacklog(asClassifier(e), rulesetNew, requestInstant.Add(time.Hour), 100, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, row := range backlog {
		offered = offered || row.ID == forged
	}
	if !offered {
		t.Fatal("an unjudged calendar reply never reaches the owed-verdict backlog")
	}
	if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), forged, OwedVerdictAsksUs, "prompts-test", dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	if !owedNow(t, e, forged) {
		t.Error("a calendar reply judged asks_us is not on the lane")
	}
	asOf := requestInstant.Add(time.Hour)
	review, _, err := storeKnowing(e).ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &asOf})
	if err != nil {
		t.Fatal(err)
	}
	reviewable := false
	for _, row := range review {
		reviewable = reviewable || ids.UUID(row.Id) == forged
	}
	if !reviewable {
		t.Error("a calendar reply judged asks_us is not offered for review")
	}
}
