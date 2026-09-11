// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Asking what a message's thread is being held for, from the sender's ledger
// row rather than from a normalized record.
//
// threadIsPrivateTx (sinkpersonal.go) answers a neighbouring question during
// capture, keyed on the record the sink is writing and on the acting seat. The
// verdict engine has neither: it runs long afterwards, as a system principal,
// holding an activity id. Same table, different way in.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadHoldsItsCounterparty reports whether the thread carrying this activity
// is under a hold that must keep the sender's contact record owner-scoped.
//
// The status set is the SAME one widenClearedSender refuses, and deliberately
// so: a thread whose held mail may not be republished is a thread whose
// counterparty may not be announced either. Two spellings of that set would
// disagree the first time a status was added, and the disagreement would be
// silent in the direction that discloses.
//
// The MESSAGE'S OWN AUDIENCE is the arm that catches every hold, and it is why
// the thread verdict alone was not enough. A message can be held by a
// [Confidential] subject marker, by a counterparty hold, or by a mailbox that
// holds everything, and none of those writes a thread-verdict row — the birth
// decision records them on the message instead. Asking only the thread ledger
// therefore answered "not held" for a confidential message and published its
// counterparty while the correspondence itself stayed shut.
//
// Only the reasons that are a HOLD, and this is the distinction the first
// version missed. A message can also be narrow because its own sender verdict
// has not come back yet ('pending_verdict') or because the mailbox holds
// everything until judged ('posture') — both are "waiting for this answer", not
// "this must stay private", and treating them as holds narrowed every ordinary
// contact the verdict was about to publish.
//
// The four named here are the real ones: a mailbox whose floor is narrower than
// the workspace, a counterparty somebody put a hold on, a [Confidential] marker,
// and a thread a classifier already judged confidential.
//
// A RESTRICTED activity is a hold in its own right, and the arm is a LEFT JOIN
// so it stands whether or not the thread carries a verdict. This is the one
// place a held row makes the answer MORE careful rather than less: the
// restricted-reader gate exists to stop a held row's content reaching a reader,
// and here the row's existence only ever narrows what gets published. Filtering
// it out would have meant a legally held message minted a workspace-visible
// contact naming its counterparty — the disclosure the hold exists to prevent.
// No activity column reaches the caller: the whole output is one boolean.
//
// A thread with no verdict row and no restriction is not a hold. Most mail is
// neither.
func ThreadHoldsItsCounterparty(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1
		    FROM activity a
		    LEFT JOIN capture_thread_verdict v ON v.thread_key = a.thread_key
		                                      AND a.thread_key <> ''
		   WHERE a.id = $1
		     AND (a.restricted_at IS NOT NULL
		          OR (a.audience <> 'workspace' AND a.audience_reason IN (
		                'workspace_floor', 'counterparty',
		                'explicitly_confidential', 'inherited_verdict'))
		          OR v.status IN ('held', 'unsure', 'held_by_owner', 'pending')))`,
		activityID).Scan(&held)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("capture: reading whether the thread is held: %w", err)
	}
	return held, nil
}
