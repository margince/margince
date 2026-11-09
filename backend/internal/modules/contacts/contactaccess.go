// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func stampContactAttachable(ctx context.Context, tx pgx.Tx, contacts []crmcontracts.Contact) error {
	rowIDs := make([]ids.UUID, len(contacts))
	for i, contact := range contacts {
		rowIDs[i] = ids.UUID(contact.Id)
	}
	permitted, err := auth.AttachableSubset(ctx, tx, contactEntity, rowIDs)
	if err != nil {
		return err
	}
	for i := range contacts {
		may := permitted[ids.UUID(contacts[i].Id)]
		contacts[i].Attachable = &may
	}
	return nil
}
