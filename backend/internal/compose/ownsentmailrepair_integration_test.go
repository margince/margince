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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	repairSeatMailbox = "seat@myco.test"
	repairFormerSelf  = "founder@previous-employer.example"
	repairBuyer       = "buyer@customer.example"
)

// repairMessage is one message as the seat's mailbox holds it. received adds
// the hop a delivering server prepends.
func repairMessage(from, messageID string, received bool) []byte {
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
	return []byte(strings.Join(lines, "\r\n"))
}

// seedReceivedMail lands one message as the seat's Gmail connection stores mail
// it was not told it sent, and places the pass's cutoff after it, as the
// migration does for mail captured before it ran.
func seedReceivedMail(t *testing.T, e *integration.Env, from, messageID string, received bool) ids.UUID {
	t.Helper()
	raw := repairMessage(from, messageID, received)
	msg, err := mailmap.Parse(raw, repairSeatMailbox)
	if err != nil {
		t.Fatalf("parsing %s: %v", messageID, err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(e.ConnectorOwnerCtx(e.AdminUser, "gmail"), msg.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("seeding %s: %v", messageID, err)
	}
	e.WsExec(t, `INSERT INTO activity_own_sent_mail_repair_cutoff (captured_before) VALUES (now() + interval '1 hour')
		ON CONFLICT (singleton) DO UPDATE SET captured_before = excluded.captured_before`)
	return ref.ID
}

// seedRepairSeat connects the seat's Gmail mailbox and a Graph mailbox granted
// for their former address, which proves that address is theirs.
func seedRepairSeat(t *testing.T, e *integration.Env) {
	t.Helper()
	connectMailbox(t, e, e.AdminUser, "gmail", repairSeatMailbox)
	connectMailbox(t, e, e.AdminUser, "graph", repairFormerSelf)
}

func connectMailbox(t *testing.T, e *integration.Env, seat ids.UUID, provider, label string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ($1, $2, 'connected', $3)`, provider, seat, label)
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
	// The replay read the row as received before the claim; it reads it again.
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant_replay WHERE activity_id = $1`, id); n != 0 {
		t.Errorf("the claimed email still carries %d participant replay markers, want it offered to the replay again", n)
	}
}

// A replay changes nothing. The marker keeps the row from being offered again,
// and judging the claimed row once more finds nothing left to turn round.
func TestTheOwnSentMailRepairReplaysWithoutASecondClaim(t *testing.T) {
	e := integration.Setup(t)
	seedRepairSeat(t, e)
	id := seedReceivedMail(t, e, repairFormerSelf, "replayed@previous-employer.example", false)

	runOwnSentMailRepair(t, e)
	runOwnSentMailRepair(t, e)

	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), ownSentMailRepairActor)
	row := capture.StoredOwnSentMail{Activity: ids.From[ids.ActivityKind](id), Seat: e.AdminUser, Sender: repairFormerSelf}
	raw := repairMessage(repairFormerSelf, "replayed@previous-employer.example", false)
	parties, err := mailmap.ParticipantsOf(raw, "")
	if err != nil {
		t.Fatalf("reading the parties: %v", err)
	}
	row.Participants = parties.Participants
	var verdict string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		verdict, err = capture.ReclaimStoredOwnSentMailTx(ctx, tx, activities.ClaimOwnSentMailTx, row, raw)
		return err
	}); err != nil {
		t.Fatalf("judging the claimed row again: %v", err)
	}

	if verdict != capture.OwnSentMailUnchanged {
		t.Errorf("judging the claimed row again says %q, want %q", verdict, capture.OwnSentMailUnchanged)
	}
	if n := repairClaims(t, e, id); n != 1 {
		t.Errorf("the replays left %d audit rows, want the one claim", n)
	}
}

func TestTheOwnSentMailRepairLeavesMailItCannotClaim(t *testing.T) {
	cases := []struct {
		name, verdict            string
		received, buyerWasErased bool
	}{
		// A delivering hop says the mailbox received it, whoever the From names.
		{"delivered from the seat's address", capture.OwnSentMailDelivered, true, false},
		// The original outlived the recipient's erasure; the claim would write it back.
		{"to an erased recipient", capture.OwnSentMailErased, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			seedRepairSeat(t, e)
			if tc.buyerWasErased {
				e.WsExec(t, `INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1)`,
					storekit.SuppressionHash(repairBuyer))
			}
			id := seedReceivedMail(t, e, repairFormerSelf, "kept@"+strings.ReplaceAll(tc.name, " ", "-")+".example", tc.received)

			runOwnSentMailRepair(t, e)

			want := storedEnds{direction: "inbound", counterparty: repairFormerSelf, verdict: tc.verdict}
			if got := storedEndsOf(t, e, id); got != want {
				t.Errorf("after the pass the mail reads %+v, want %+v", got, want)
			}
			if n := repairClaims(t, e, id); n != 0 {
				t.Errorf("the pass wrote %d audit rows on mail it may not claim, want 0", n)
			}
		})
	}
}

// Mail the pass never offers: its sender is not an address the seat alone has
// proven, or it arrived after the pass's cutoff.
func TestTheOwnSentMailRepairDoesNotOfferMailOutsideItsReach(t *testing.T) {
	cases := []struct {
		name  string
		setUp func(t *testing.T, e *integration.Env)
	}{
		{"a stranger's address", func(t *testing.T, e *integration.Env) {
			connectMailbox(t, e, e.AdminUser, "gmail", repairSeatMailbox)
		}},
		// A declaration is the seat's own say-so; without the provider's sent
		// filing it is not enough to turn mail round.
		{"an address the seat only declared", func(t *testing.T, e *integration.Env) {
			connectMailbox(t, e, e.AdminUser, "gmail", repairSeatMailbox)
			if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.Admin(), capture.IdentityKindAddress, repairFormerSelf); err != nil {
				t.Fatalf("declaring %s: %v", repairFormerSelf, err)
			}
		}},
		{"an address two seats proved", func(t *testing.T, e *integration.Env) {
			seedRepairSeat(t, e)
			connectMailbox(t, e, e.Rep1, "graph", repairFormerSelf)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			tc.setUp(t, e)
			id := seedReceivedMail(t, e, repairFormerSelf, "unreached@"+strings.ReplaceAll(tc.name, " ", "-")+".example", false)

			runOwnSentMailRepair(t, e)

			want := storedEnds{direction: "inbound", counterparty: repairFormerSelf}
			if got := storedEndsOf(t, e, id); got != want {
				t.Errorf("after the pass the mail reads %+v, want %+v and no verdict", got, want)
			}
		})
	}
	t.Run("captured after the cutoff", func(t *testing.T) {
		e := integration.Setup(t)
		seedRepairSeat(t, e)
		id := seedReceivedMail(t, e, repairFormerSelf, "after-cutoff@previous-employer.example", false)
		e.WsExec(t, `UPDATE activity_own_sent_mail_repair_cutoff SET captured_before = now() - interval '1 hour'`)

		runOwnSentMailRepair(t, e)

		if got := storedEndsOf(t, e, id); got.direction != "inbound" || got.verdict != "" {
			t.Errorf("mail captured after the cutoff reads %+v, want it left to live capture", got)
		}
	})
}

// Standing gained later brings the mail into reach: connecting the mailbox for
// the former address proves it.
func TestMailFromAnAddressTheSeatProvesLaterIsClaimed(t *testing.T) {
	e := integration.Setup(t)
	connectMailbox(t, e, e.AdminUser, "gmail", repairSeatMailbox)
	id := seedReceivedMail(t, e, repairFormerSelf, "proved-later@previous-employer.example", false)

	runOwnSentMailRepair(t, e)
	if got := storedEndsOf(t, e, id); got.direction != "inbound" {
		t.Fatalf("before the address was proven the mail reads %+v, want inbound", got)
	}

	connectMailbox(t, e, e.AdminUser, "graph", repairFormerSelf)
	runOwnSentMailRepair(t, e)

	want := storedEnds{direction: "outbound", counterparty: repairBuyer, verdict: capture.OwnSentMailClaimed, attested: true}
	if got := storedEndsOf(t, e, id); got != want {
		t.Errorf("after the address was proven the mail reads %+v, want %+v", got, want)
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
