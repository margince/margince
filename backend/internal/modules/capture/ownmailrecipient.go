// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Raw address evidence keeps parsed recipients distinct from legacy delivery stamps.
// Own identities never pass through the external-contact admission path.
func stampOwnMailRecipient(ctx context.Context, tx pgx.Tx, id ids.ActivityID, fields ActivityFields) error {
	if fields.Kind != kindEmail || fields.Envelope == nil {
		return nil
	}
	own, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return err
	}
	return stampMailRecipients(ctx, tx, id, fields.Envelope, own.Covers)
}

func stampMailRecipients(ctx context.Context, tx pgx.Tx, id ids.ActivityID, envelope *MailEnvelope, belongs func(string) bool) error {
	if envelope == nil {
		return nil
	}
	for _, address := range envelope.OpenRecipients {
		if !belongs(address) {
			continue
		}
		role := connector.ParticipantRoleCC
		if slices.Contains(envelope.DirectRecipients, address) {
			role = connector.ParticipantRoleTo
		}
		if err := insertParticipant(ctx, tx, id, role, nil, nil, address, ""); err != nil {
			return err
		}
	}
	return nil
}
