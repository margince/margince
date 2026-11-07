// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// lockCorrespondenceForAttestedSend serializes this insert against the verdict
// that reads whether the workspace corresponds with the counterparty.
//
// Attested outbound is the one write that turns that answer from no to yes, and
// the verdict acts on it inside its own transaction. Capture's sink takes the
// same key on the rows a connector files.
func lockCorrespondenceForAttestedSend(
	ctx context.Context, tx pgx.Tx, in LogActivityInput, counterparty string,
) error {
	if !in.CounterpartyOutboundAttested || counterparty == "" {
		return nil
	}
	return storekit.LockWriteIdentity(ctx, tx, correspondence.LockEntity,
		mailidentity.CorrespondenceLockKey(counterparty))
}
