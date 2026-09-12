// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What a `classified` mailbox does with mail from a sender it has ALREADY had
// judged.
//
// The release that re-opens held mail answers the messages that arrived before
// the verdict. This is the other half: once the question is settled, the next
// message from that sender must not be held for it again — nothing re-asks a
// question the ledger considers answered, so a message held here stays held for
// good.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailCapturedAfterAClearedVerdictIsBornOpen(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	clearSender(t, e, "chi@fvhospital.example")

	sync(t, email("chi@fvhospital.example", "Chi Phan", captureOwner,
		"cleared-1@fvhospital.example", ""))

	activityID := newestActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "workspace" || reason != "" {
		t.Errorf("mail from a sender already judged a real contact was born %q / %q, want workspace: "+
			"the mailbox held it for a question that has an answer, and nothing re-asks it",
			got, reason)
	}
}

// The rung only substitutes the mailbox's own posture. Everything stricter ran
// before it and still decides: a seat's counterparty hold is their decision
// about this correspondent, and no verdict about the sender speaks for it.
func TestAClearedSenderIsStillHeldByThisSeatsOwnHold(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	clearSender(t, e, "anwalt@studiolegal.example")
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")

	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"cleared-2@studiolegal.example", ""))

	activityID := newestActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Errorf("a message under this seat's own counterparty hold was born %q / %q, want "+
			"participants / counterparty: a verdict about the sender overrode a hold that was "+
			"never about the verdict", got, reason)
	}
}

// A sender judged real but not a CONTACT leaves the question the posture asked
// unanswered — an advisor's record is deliberately kept to its owner, and their
// mail is exactly the mail a classified mailbox means to hold.
func TestAnAdvisorVerdictDoesNotOpenTheNextMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	settleSender(t, e, "anwalt@kanzlei.example", capturemod.PendingStatusReal, capturemod.KindAdvisor)

	sync(t, email("anwalt@kanzlei.example", "Dr. Legal", captureOwner,
		"cleared-3@kanzlei.example", ""))

	activityID := newestActivityID(t, e)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Errorf("mail from an advisor was born %q: the rung asked for status alone rather than "+
			"whether a contact was behind the address", got)
	}
}

// clearSender records the settled verdict that a real contact is behind this
// address, which is what the birth ladder reads.
func clearSender(t *testing.T, e *integration.SearchEnv, email string) {
	t.Helper()
	settleSender(t, e, email, capturemod.PendingStatusReal, capturemod.KindContact)
}

// settleSender writes one settled ledger row, against the earlier message that
// raised the question. Written directly because what the scenario needs is the
// ledger state a verdict pass leaves behind, and driving the classifier would
// test the classifier instead.
func settleSender(t *testing.T, e *integration.SearchEnv, email, status, kind string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// The ledger row names the message that first asked about this sender,
		// so one has to exist for it to point at.
		asked := ids.NewV7()
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email)
			VALUES ($1, 'email', 'the message that asked', 'body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', $3)`,
			asked, "asked-"+asked.String(), email); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			  (id, email, domain, display_name, activity_id, owner_id, status, kind, resolved_at)
			VALUES ($1, $2, split_part($2, '@', 2), 'Settled Sender', $3, $4, $5, $6, now())`,
			ids.NewV7(), email, asked, e.Rep1, status, kind)
		return err
	}); err != nil {
		t.Fatalf("settling the sender's verdict: %v", err)
	}
}
