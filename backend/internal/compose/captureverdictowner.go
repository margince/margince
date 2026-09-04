// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the SEAT said about a sender, held apart from what the model thinks.
//
// Its own file because the two callers ask it for different reasons and at
// different moments: judgeOne asks BEFORE spending a model call, because a
// person who has answered has answered; apply asks again inside its own
// transaction, because the first read is minutes old by then and a decision
// taken in between was being overruled by a stale one.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// ownerDecided answers whether the mailbox owner already settled this sender,
// and which kind their decision amounts to.
//
// `business` becomes `person`: the owner is saying this is somebody the CRM
// should hold, which is the one kind that creates a record. `keep_out` becomes
// `spam`, the noise kind whose effects — hide the mail, suppress the domain —
// are what "keep this out for good" means. Neither invents a new kind: the
// ledger's vocabulary is closed: a decision spelled outside it would sit in a
// column every downstream reader parses against a fixed set, and be skipped.
func (e *CounterpartyVerdictEngine) ownerDecided(ctx context.Context, row capture.PendingCounterparty) (bool, string, error) {
	var decision string
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		decision, err = capture.OverrideForTx(ctx, tx, row.OwnerID, row.Email)
		return err
	}); err != nil {
		return false, "", err
	}
	kind, decided := kindForOverride(decision)
	return decided, kind, nil
}

// kindForOverride reads a sender decision as the kind it settles the sender at,
// answering whether the owner has spoken at all.
//
// One spelling, because two callers now ask it: judgeOne, deciding whether to
// spend a model call, and apply, checking whether the answer changed underneath
// one. A second copy would let those two disagree about what a decision means,
// and the disagreement would show up as a verdict that contradicts the person
// who set it.
func kindForOverride(decision string) (kind string, decided bool) {
	switch decision {
	case capture.OverrideBusiness:
		return capture.KindPerson, true
	case capture.OverrideKeepOut:
		return capture.KindSpam, true
	}
	return "", false
}
