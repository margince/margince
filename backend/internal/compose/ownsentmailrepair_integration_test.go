// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The pass that re-reads mail a seat sent from another address of theirs,
// stored as received before capture read it as outbound.
//
// Every message is seeded through the real mapper and Sink, unfiled as sent,
// which is the shape such mail was stored in: inbound, with the seat's other
// address as its counterparty. The pass runs through the participant backfill
// worker, the wiring production runs it under.

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	repairSeatMailbox = "seat@myco.test"
	repairFormerSelf  = "founder@previous-employer.example"
	repairBuyer       = "buyer@customer.example"
)

// seedReceivedMail lands one message as the seat's Gmail connection stores mail
// it was not told it sent. received adds the hop a delivering server prepends.
func seedReceivedMail(t *testing.T, e *integration.Env, from, messageID string, received bool) ids.UUID {
	t.Helper()
	lines := []string{
		"From: Founder <" + from + ">",
		"To: " + repairBuyer,
		"Cc: " + repairSeatMailbox,
		"Subject: following up",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"Content-Type: text/plain", "", "hello", "",
	}
	if received {
		lines = append([]string{"Received: from mx.customer.example by mx.google.com; Wed, 04 Jun 2026 08:00:01 +0000"}, lines...)
	}
	raw := []byte(strings.Join(lines, "\r\n"))
	msg, err := mailmap.Parse(raw, repairSeatMailbox)
	if err != nil {
		t.Fatalf("parsing %s: %v", messageID, err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(e.ConnectorOwnerCtx(e.AdminUser, "gmail"), msg.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("seeding %s: %v", messageID, err)
	}
	return ref.ID
}

// seedRepairSeat connects the seat's Gmail mailbox and declares their former
// address as theirs.
func seedRepairSeat(t *testing.T, e *integration.Env) {
	t.Helper()
	connectRepairSeat(t, e)
	declareFormerSelf(t, e)
}

func connectRepairSeat(t *testing.T, e *integration.Env) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ('gmail', $1, 'connected', $2)`, e.AdminUser, repairSeatMailbox)
}

func declareFormerSelf(t *testing.T, e *integration.Env) {
	t.Helper()
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.Admin(), capture.IdentityKindAddress, repairFormerSelf); err != nil {
		t.Fatalf("declaring %s: %v", repairFormerSelf, err)
	}
}

func runOwnSentMailRepair(t *testing.T, e *integration.Env) {
	t.Helper()
	if _, err := newParticipantBackfillWorker(e.Pool, slog.Default()).backfillWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("running the participant backfill: %v", err)
	}
}

type storedEnds struct {
	direction, counterparty, verdict string
	attested                         bool
}

func storedEndsOf(t *testing.T, e *integration.Env, id ids.UUID) storedEnds {
	t.Helper()
	var ends storedEnds
	ends.direction = e.WsScalar(t, `SELECT direction FROM activity WHERE id = $1`, id)
	ends.counterparty = e.WsScalar(t, `SELECT counterparty_email FROM activity WHERE id = $1`, id)
	ends.attested = e.WsScalar(t, `SELECT counterparty_outbound_attested::text FROM activity WHERE id = $1`, id) == "true"
	ends.verdict = e.WsScalar(t, `SELECT coalesce((SELECT outcome FROM activity_own_sent_mail_repair WHERE activity_id = $1), '')`, id)
	return ends
}

func repairClaims(t *testing.T, e *integration.Env, id ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'activity' AND entity_id = $1 AND actor_id = $2`, id, ownSentMailRepairActor)
}

func TestMailTheSeatWroteFromAFormerAddressIsReadAgainAsTheirs(t *testing.T) {
	e := integration.Setup(t)
	seedRepairSeat(t, e)
	id := seedReceivedMail(t, e, repairFormerSelf, "former-sent@previous-employer.example", false)
	if got := storedEndsOf(t, e, id); got.direction != "inbound" || got.counterparty != repairFormerSelf {
		t.Fatalf("seeded as %+v, want the pre-fix shape: inbound from %s", got, repairFormerSelf)
	}

	runOwnSentMailRepair(t, e)

	want := storedEnds{direction: "outbound", counterparty: repairBuyer, verdict: capture.OwnSentMailClaimed, attested: true}
	if got := storedEndsOf(t, e, id); got != want {
		t.Errorf("after the pass the mail reads %+v, want %+v", got, want)
	}
	if n := repairClaims(t, e, id); n != 1 {
		t.Errorf("the claim left %d audit rows under the pass's name, want 1", n)
	}
	if seat := e.WsScalar(t, `SELECT on_behalf_of::text FROM audit_log WHERE entity_id = $1 AND actor_id = $2`,
		id, ownSentMailRepairActor); seat != e.AdminUser.String() {
		t.Errorf("the claim's audit row speaks for %q, want the seat whose mailbox stored it", seat)
	}
}

// A replay changes nothing: the marker keeps the row from being offered again,
// and with the markers gone the row, outbound now, is not offered at all.
func TestTheOwnSentMailRepairReplaysWithoutASecondClaim(t *testing.T) {
	e := integration.Setup(t)
	seedRepairSeat(t, e)
	id := seedReceivedMail(t, e, repairFormerSelf, "replayed@previous-employer.example", false)

	runOwnSentMailRepair(t, e)
	runOwnSentMailRepair(t, e)
	e.WsExec(t, `DELETE FROM activity_own_sent_mail_repair`)
	runOwnSentMailRepair(t, e)

	if n := repairClaims(t, e, id); n != 1 {
		t.Errorf("three passes left %d audit rows, want the one claim", n)
	}
	if got := storedEndsOf(t, e, id); got.direction != "outbound" || got.counterparty != repairBuyer {
		t.Errorf("after the replay the mail reads %+v, want outbound to %s", got, repairBuyer)
	}
}

func TestTheOwnSentMailRepairLeavesMailItCannotClaim(t *testing.T) {
	cases := []struct {
		name, from, verdict      string
		received, buyerWasErased bool
	}{
		// A delivering hop says the mailbox received it, whoever the From names.
		{"delivered from the seat's address", repairFormerSelf, capture.OwnSentMailDelivered, true, false},
		// A From line is the sender's text; an address the seat never held is a stranger's.
		{"from a stranger", "someone@elsewhere.example", capture.OwnSentMailNotTheSeats, false, false},
		// The original outlived the recipient's erasure; the claim would write it back.
		{"to an erased recipient", repairFormerSelf, capture.OwnSentMailErased, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			seedRepairSeat(t, e)
			if tc.buyerWasErased {
				e.WsExec(t, `INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1)`,
					storekit.SuppressionHash(repairBuyer))
			}
			id := seedReceivedMail(t, e, tc.from, "kept@"+strings.ReplaceAll(tc.name, " ", "-")+".example", tc.received)

			runOwnSentMailRepair(t, e)

			want := storedEnds{direction: "inbound", counterparty: tc.from, verdict: tc.verdict}
			if got := storedEndsOf(t, e, id); got != want {
				t.Errorf("after the pass the mail reads %+v, want %+v", got, want)
			}
			if n := repairClaims(t, e, id); n != 0 {
				t.Errorf("the pass wrote %d audit rows on mail it may not claim, want 0", n)
			}
		})
	}
}

// A verdict about the seat's addresses was about the addresses they held then:
// declaring the former address afterwards brings its mail back to be judged.
func TestMailFromAnAddressTheSeatDeclaresLaterIsJudgedAgain(t *testing.T) {
	e := integration.Setup(t)
	connectRepairSeat(t, e)
	id := seedReceivedMail(t, e, repairFormerSelf, "declared-later@previous-employer.example", false)

	runOwnSentMailRepair(t, e)
	if got := storedEndsOf(t, e, id); got.verdict != capture.OwnSentMailNotTheSeats {
		t.Fatalf("before the declaration the pass recorded %q, want %q", got.verdict, capture.OwnSentMailNotTheSeats)
	}

	declareFormerSelf(t, e)
	runOwnSentMailRepair(t, e)

	want := storedEnds{direction: "outbound", counterparty: repairBuyer, verdict: capture.OwnSentMailClaimed, attested: true}
	if got := storedEndsOf(t, e, id); got != want {
		t.Errorf("after the declaration the mail reads %+v, want %+v", got, want)
	}
}

// A row captured before provenance named the seat carries a bare
// `connector:gmail` and no link to its original; it is offered all the same.
func TestTheOwnSentMailRepairReachesARowFromBeforeProvenanceNamedTheSeat(t *testing.T) {
	e := integration.Setup(t)
	seedRepairSeat(t, e)
	id := seedReceivedMail(t, e, repairFormerSelf, "legacy@previous-employer.example", false)
	e.WsExec(t, `UPDATE activity SET captured_by = 'connector:gmail', raw_capture_id = NULL WHERE id = $1`, id)

	runOwnSentMailRepair(t, e)

	if got := storedEndsOf(t, e, id); got.direction != "outbound" || got.verdict != capture.OwnSentMailClaimed {
		t.Errorf("the legacy row reads %+v, want it claimed as outbound", got)
	}
}
