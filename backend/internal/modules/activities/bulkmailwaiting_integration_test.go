// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Mail that carried a List-Unsubscribe header is a newsletter. It waits on no
// reply, answers nothing, and is no request until a human takes it on. These
// tests stamp bulk_mail_attested by hand; the compose lane stamps it through
// real capture.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *loadEnv) stampBulk(t *testing.T, activity ids.UUID) {
	t.Helper()
	e.exec(t, `UPDATE activity SET bulk_mail_attested = true WHERE id = $1`, activity)
}

func TestOurOwnNewsletterAwaitsNoReply(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	newsletter, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	e.stampBulk(t, newsletter)
	written, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)

	if awaited(t, s, e, newsletter) {
		t.Error("the reader's own newsletter waits on a reply")
	}
	if !awaited(t, s, e, written) {
		t.Error("the written message beside it is not a follow-up, so the case above proves nothing")
	}
}

func TestANewsletterDoesNotAnswerOurMessage(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	contact := e.customerAt(t, "prospect")
	sent, thread := e.sentTo(t, contact, "", 3*24*time.Hour)
	e.afterwards(t, contact, "email", "inbound", thread)
	e.exec(t, `UPDATE activity SET bulk_mail_attested = true WHERE thread_key = $1 AND direction = 'inbound'`, thread)

	if !awaited(t, s, e, sent) {
		t.Error("a newsletter on the thread settled the reader's follow-up")
	}
}

// A classifier's asks_us verdict alone does not put a newsletter on the
// waiting lane. A human who takes it on as a request does.
func TestANewsletterIsOwedAReplyOnlyOnceAHumanAcceptsIt(t *testing.T) {
	e := setupLoad(t)
	newsletter := seedCalendarMailSaying(t, e, "", OwedVerdictAsksUs, "Our spring offer: reply to book a demo.")
	e.stampBulk(t, newsletter)
	written := seedCalendarMailSaying(t, e, "", OwedVerdictAsksUs, "Could you send the offer by Friday?")

	if owedNow(t, e, newsletter) {
		t.Error("a newsletter the classifier says asks us is owed a reply")
	}
	if !owedNow(t, e, written) {
		t.Fatal("a written request the classifier says asks us is not owed, so the case above proves nothing")
	}

	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	reader := principal.WithActor(e.as(), actor)
	if _, _, err := storeKnowing(e).LogActivity(reader, LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &newsletter}); err != nil {
		t.Fatal(err)
	}
	if !badgeNeedsReply(t, e, newsletter) {
		t.Error("a newsletter a rep took on as a request is not owed a reply")
	}
}
