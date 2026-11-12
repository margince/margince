// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// markBodyRead gives a seeded mail the body state `completeness`. Mail captured
// before the state was recorded reads `unassessed`, and the data task's writer
// moves it from there to what its original shows.
func markBodyRead(t *testing.T, e *loadEnv, mail ids.UUID, completeness string) {
	t.Helper()
	if completeness == "complete" {
		return
	}
	e.exec(t, `UPDATE activity SET body_completeness = 'unassessed' WHERE id = $1`, mail)
	if completeness == "unassessed" {
		return
	}
	var body string
	if err := e.owner.QueryRow(t.Context(), `SELECT body FROM activity WHERE id = $1`, mail).Scan(&body); err != nil {
		t.Fatal(err)
	}
	system := principal.WithActor(principal.WithWorkspaceID(t.Context(), e.ws),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:mail_body_completeness"})
	err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
		changed, err := AssessMailBodiesTx(system, tx, []MailBodyAssessment{{Activity: mail, Completeness: completeness, Body: body}})
		if err == nil && changed != 1 {
			t.Fatalf("assessed %d mails, want 1", changed)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// reviewAsAsksUs is a human correcting the classifier to asks_us through the
// reviewed-classification writer.
func reviewAsAsksUs(t *testing.T, e *loadEnv, mail ids.UUID) {
	t.Helper()
	reviewer := e.asSeat(e.rep)
	err := database.WithWorkspaceTx(reviewer, e.pool, func(tx pgx.Tx) error {
		expected, err := OwedReviewTx(reviewer, tx, mail, 4000, 4000)
		if err != nil {
			return err
		}
		applied, err := ApplyReviewedOwedVerdictTx(reviewer, tx, expected, OwedVerdictAsksUs, "current-rules", "classification_changed", ids.NewV7(), 4000, 4000)
		if err == nil && !applied {
			t.Fatal("the reviewed verdict was not applied")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func offeredForReview(t *testing.T, e *loadEnv, mail ids.UUID, at time.Time) bool {
	t.Helper()
	rows, _, err := storeKnowing(e).ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &at})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == mail {
			return true
		}
	}
	return false
}

// A machine's asks_us verdict is offered as a reply owed only on a mail read in
// full. A mail whose text cannot be checked is neither listed nor taken.
func TestOnlyAMailReadInFullIsOfferedAsAReplyOwed(t *testing.T) {
	e := setupLoad(t)
	at := requestInstant.Add(time.Hour)
	for _, completeness := range []string{"complete", "partial", "unverifiable", "unassessed"} {
		t.Run(completeness, func(t *testing.T) {
			mail := seedEmailRequest(t, e, "Please send the price list", "commitment", OwedVerdictAsksUs)
			markBodyRead(t, e, mail, completeness)
			readable := completeness == "complete"
			if got := offeredForReview(t, e, mail, at); got != readable {
				t.Fatalf("offered for review = %t, want %t", got, readable)
			}
			_, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"})
			if readable && err != nil {
				t.Fatalf("taking a request read in full: %v", err)
			}
			if !readable && !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("taking a request whose text cannot be checked = %v, want a conflict", err)
			}
		})
	}
}

// A human who reviewed the classification read the mail themselves, so their
// verdict stands on a mail Margince can no longer check.
func TestAHumanReviewedVerdictIsOfferedOnAMailThatCannotBeChecked(t *testing.T) {
	e := setupLoad(t)
	mail := seedEmailRequest(t, e, "Could you send the contract?", "commitment", OwedVerdictInformsUs)
	reviewer := e.asSeat(e.rep)
	reviewAsAsksUs(t, e, mail)
	markBodyRead(t, e, mail, "unverifiable")
	if !offeredForReview(t, e, mail, requestInstant.Add(time.Hour)) {
		t.Fatal("a human-reviewed verdict on an unverifiable mail left the review list")
	}
	if _, _, err := storeKnowing(e).LogActivity(reviewer, LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"}); err != nil {
		t.Fatalf("taking a human-reviewed request: %v", err)
	}
}

// A reminder taken before the mail turned unverifiable stays its holder's: the
// holder can still replay the take.
func TestAReminderTakenBeforeTheTextWasLostStillReplays(t *testing.T) {
	e := setupLoad(t)
	mail := seedEmailRequest(t, e, "Please send slots", "meeting", OwedVerdictAsksUs)
	holder := e.asSeat(e.rep)
	first, _, err := storeKnowing(e).LogActivity(holder, LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	markBodyRead(t, e, mail, "unverifiable")
	again, _, err := storeKnowing(e).LogActivity(holder, LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"})
	if err != nil {
		t.Fatalf("replaying a held reminder: %v", err)
	}
	if again.Id != first.Id {
		t.Fatal("the replay wrote a second reminder")
	}
}

// An open reminder on a request the settlement pass will never judge says so,
// and names our last reply in the conversation.
func TestAReminderMarginceCannotCheckSaysSoAndNamesOurReply(t *testing.T) {
	e := setupLoad(t)
	mail := seedEmailRequest(t, e, "Please send the offer", "commitment", OwedVerdictAsksUs)
	taken, _, err := storeKnowing(e).LogActivity(e.asSeat(e.other), LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	task := ids.UUID(taken.Id)
	reply := ourReplyTo(t, e, mail, requestInstant.Add(time.Hour))
	at := requestInstant.Add(2 * time.Hour)

	checks, err := storeKnowing(e).UncheckableRequestReminders(e.asSeat(e.other), []ids.UUID{task}, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := checks[task]; found {
		t.Fatal("a request read in full carries a request check")
	}

	markBodyRead(t, e, mail, "unverifiable")
	checks, err = storeKnowing(e).UncheckableRequestReminders(e.asSeat(e.other), []ids.UUID{task}, at)
	if err != nil {
		t.Fatal(err)
	}
	check, found := checks[task]
	if !found || check.State != crmcontracts.TaskRequestCheckStateTextNotChecked {
		t.Fatalf("request check = %+v, want text_not_checked", checks)
	}
	if check.OurLastReply == nil || ids.UUID(check.OurLastReply.ActivityId) != reply {
		t.Fatalf("our last reply = %+v, want %s", check.OurLastReply, reply)
	}
}

// ourReplyTo is our attested reply in the request's conversation.
func ourReplyTo(t *testing.T, e *loadEnv, request ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	var key string
	if err := e.owner.QueryRow(t.Context(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
		t.Fatal(err)
	}
	subject, body, direction := "Re: offer", "Here is the offer.", "outbound"
	reply, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: key, Source: "test", OccurredAt: &at,
		CounterpartyEmail: requestCounterparty, CounterpartyOutboundAttested: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(reply.Id)
}

// Our last reply is our own mail. A call, a held meeting or the customer's
// later mail answers the request too, and none of them is a reply we sent.
func TestOurLastReplyIsNeverACallAMeetingOrTheirMail(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	mail := seedEmailRequestFiledUnder(t, e, "Please send the offer", "commitment", OwedVerdictAsksUs, requestCounterparty,
		[]ActivityLinkInput{{EntityType: "contact", EntityID: contact}}, mailParticipants{})
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address, contact_id) VALUES ($1, 'from', $2, $3)`,
		mail, requestCounterparty, contact)
	taken, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", RequestActivityID: &mail, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	task := ids.UUID(taken.Id)
	markBodyRead(t, e, mail, "unverifiable")
	at := requestInstant.Add(24 * time.Hour)
	lastReply := func() *crmcontracts.TaskRequestCheckReply {
		t.Helper()
		checks, err := storeKnowing(e).UncheckableRequestReminders(e.asSeat(e.rep), []ids.UUID{task}, at)
		if err != nil {
			t.Fatal(err)
		}
		check, found := checks[task]
		if !found {
			t.Fatal("the reminder lost its request check")
		}
		return check.OurLastReply
	}

	call := e.touch(t, "call", nil, contact, requestInstant.Add(time.Hour))
	if newest := newestSettlementAnswer(t, e, mail, at); newest != call {
		t.Fatalf("newest answer = %s, want the call %s", newest, call)
	}
	if reply := lastReply(); reply != nil {
		t.Fatalf("a call was named as our last reply: %+v", reply)
	}

	reply := ourReplyTo(t, e, mail, requestInstant.Add(2*time.Hour))
	held := "held"
	meeting := e.touch(t, "meeting", &held, contact, requestInstant.Add(3*time.Hour))
	theirs := theirLaterMail(t, e, mail, requestInstant.Add(4*time.Hour))
	if newest := newestSettlementAnswer(t, e, mail, at); newest != theirs {
		t.Fatalf("newest answer = %s, want their mail %s (meeting %s)", newest, theirs, meeting)
	}
	if got := lastReply(); got == nil || ids.UUID(got.ActivityId) != reply {
		t.Fatalf("our last reply = %+v, want our mail %s", got, reply)
	}
}

// touch is a logged call or meeting with the contact, filed under them.
func (e *loadEnv) touch(t *testing.T, kind string, status *string, contact ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	row, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), LogActivityInput{
		Kind: kind, Subject: &kind, Source: "manual", OccurredAt: &at, MeetingStatus: status,
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(row.Id)
}

// theirLaterMail is the customer's own later mail in the conversation, with
// the sender authentication capture records for it.
func theirLaterMail(t *testing.T, e *loadEnv, request ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	var key string
	if err := e.owner.QueryRow(t.Context(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
		t.Fatal(err)
	}
	subject, body, direction := "Re: offer", "Thanks, got it.", "inbound"
	row, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: key, Source: "test", OccurredAt: &at, CounterpartyEmail: requestCounterparty,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE activity SET captured_by = 'connector:gmail', inbound_sender_authenticated = true WHERE id = $1`, row.Id)
	return ids.UUID(row.Id)
}

// newestSettlementAnswer is the answer the settlement pass would judge
// through, read by its own statement.
func newestSettlementAnswer(t *testing.T, e *loadEnv, request ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	var newest *ids.UUID
	if err := e.owner.QueryRow(t.Context(), `SELECT (`+newestAnswerSince+`) FROM activity a WHERE a.id = $2`, at, request).Scan(&newest); err != nil {
		t.Fatal(err)
	}
	if newest == nil {
		return ids.UUID{}
	}
	return *newest
}

func citedSettled(t *testing.T, e *loadEnv, mail ids.UUID, at time.Time) bool {
	t.Helper()
	reader := e.asSeat(e.rep)
	var settled map[ids.UUID]bool
	err := database.WithWorkspaceTx(reader, e.pool, func(tx pgx.Tx) error {
		var err error
		settled, err = SettledCitations(reader, tx, []ids.UUID{mail}, nil, at)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return settled[mail]
}

// A cited request we answered stays open only while its asks_us verdict
// stands. On a mail Margince cannot check, the answer settles the citation,
// unless a human reviewed the verdict.
func TestACitedRequestStaysOpenOnlyWhileItsVerdictStands(t *testing.T) {
	e := setupLoad(t)
	at := requestInstant.Add(24 * time.Hour)
	for _, completeness := range []string{"complete", "partial", "unverifiable"} {
		t.Run(completeness, func(t *testing.T) {
			mail := seedEmailRequest(t, e, "Please send the price list", "commitment", OwedVerdictAsksUs)
			ourReplyTo(t, e, mail, requestInstant.Add(time.Hour))
			markBodyRead(t, e, mail, completeness)
			if got, want := citedSettled(t, e, mail, at), completeness != "complete"; got != want {
				t.Fatalf("cited request settled = %t, want %t", got, want)
			}
		})
	}
	t.Run("human-reviewed", func(t *testing.T) {
		mail := seedEmailRequest(t, e, "Could you send the contract?", "commitment", OwedVerdictInformsUs)
		reviewAsAsksUs(t, e, mail)
		ourReplyTo(t, e, mail, requestInstant.Add(time.Hour))
		markBodyRead(t, e, mail, "unverifiable")
		if citedSettled(t, e, mail, at) {
			t.Fatal("a human-reviewed request was settled by a reply alone")
		}
	})
}
