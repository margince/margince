// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// counterpartyFor answers who a message being logged was with.
//
// A caller that set the column outright is obeyed — capture's own writes arrive
// that way, having derived it from the message they hold. An importer states
// the addresses instead, and who the message was WITH follows from those and
// its direction.
func counterpartyFor(ctx context.Context, tx pgx.Tx, in LogActivityInput) (string, error) {
	if in.CounterpartyEmail != "" {
		return in.CounterpartyEmail, nil
	}
	return deriveImportedCounterparty(ctx, tx, in)
}

// deriveImportedCounterparty answers who an imported message was with.
//
// It runs under the write rather than in the mapping because the answer needs
// the sending side's own addresses, and those are a read. An outbound message
// is with the first recipient who is NOT us: a sender who copies themselves is
// on their own To line, and taking the first recipient blindly would record the
// workspace as its own counterparty, which reads as correspondence with nobody.
//
// The owner set is the acting human's own address. Capture asks the mailbox it
// synced from; this path has no mailbox, so it asks who is writing. When the
// principal names no human — an extension's core write, a system caller — the
// set is empty, every address is foreign, and the first recipient stands. That
// is the answer capture reaches for a mailbox whose owner identity it has not
// learned yet, and it fails toward naming a real correspondent rather than none.
func deriveImportedCounterparty(ctx context.Context, tx pgx.Tx, in LogActivityInput) (string, error) {
	if in.Direction == nil || (in.EmailFrom == "" && len(in.EmailTo) == 0) {
		return "", nil
	}
	owned, err := actingHumanAddresses(ctx, tx)
	if err != nil {
		return "", err
	}
	return importedCounterparty(*in.Direction, mailParticipants{
		From: in.EmailFrom,
		To:   in.EmailTo,
		Cc:   in.EmailCc,
	}, func(addr string) bool { return owned[addr] }), nil
}

// actingHumanAddresses is the set of addresses that count as our own side for
// the caller writing this row.
//
// An agent or system principal has no mailbox of its own, so the set is empty
// and every address on the message belongs to somebody else — correct, because
// neither is a party to the correspondence it is filing.
func actingHumanAddresses(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	none := map[string]bool{}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return none, nil
	}
	// Deliberately NOT gated on liveness: whose address this is does not change
	// when they leave, and a colleague importing their own old mail after
	// deactivation must still not be recorded as its counterparty. The id is
	// the caller's own, already authenticated — this only asks what it is
	// called.
	var email string
	err := tx.QueryRow(ctx,
		`SELECT lower(email) FROM app_user WHERE id = $1`,
		actor.UserID).Scan(&email)
	if err != nil {
		// A principal naming a user this transaction cannot see is not an error
		// to fail the write on: it means we cannot tell which side is ours, and
		// the derivation below already treats an unknown side as foreign.
		if errors.Is(err, pgx.ErrNoRows) {
			return none, nil
		}
		return nil, fmt.Errorf("activities: acting user address: %w", err)
	}
	return map[string]bool{email: true}, nil
}
