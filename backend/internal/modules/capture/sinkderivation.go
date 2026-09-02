// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether a counterparty derivation is possible at all, before the tier ladder
// asks what to do about it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// derivationStart settles whether a derivation is possible at all and builds the
// two values the ladder works on. It reports ok=false when nothing can be
// derived — no resolver wired, no counterparty address, or no granting human.
//
// The last of those is the one with teeth (RC-8): a capture connector always
// acts for a human, and with no owner nothing can honestly own the created rows.
// The ACTIVITY still stands — refusing the derivation is the honest answer,
// where failing the capture would throw away a message we successfully read — so
// the fault is recorded for the link_reconcile sweep and creation is skipped.
func (s *Sink) derivationStart(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, cp connector.Counterparty, activityID ids.UUID) (dispositionRow, counterpartyDecision, bool, error) {
	if s.ensurer == nil || cp.Email == "" {
		return dispositionRow{}, counterpartyDecision{}, false, nil
	}
	actor, owner := capturePrincipal(ctx)
	if owner.IsZero() {
		// A fault, and one that reaches no other fault path: this returns
		// ok=false with no error, so decideCounterpartyGuarded's arm never sees
		// it and the message would otherwise trace as an ordinary capture.
		return dispositionRow{}, counterpartyDecision{}.traced(TraceFault, TraceReasonNoGrantingHuman), false,
			s.logBreadcrumbTx(ctx, tx, "capture_ensure_fault", rec, "no granting human on the connector principal")
	}
	row := dispositionRow{
		Email: cp.Email, Domain: cp.Domain, DisplayName: cp.DisplayName,
		ActivityID: activityID, OwnerID: owner,
	}
	return row, counterpartyDecision{owner: owner, capturedBy: actor.ID}, true, nil
}
