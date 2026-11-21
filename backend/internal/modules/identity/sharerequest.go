// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ShareWithRequesterTx shares a record with the colleague who asked for it:
// write, or read for a seat that may not write. The share is checked against
// the caller's grants as if they pressed Share.
func (s *Service) ShareWithRequesterTx(ctx context.Context, tx pgx.Tx, recordType string, recordID, requester ids.UUID) error {
	access, err := ShareAccessForTx(ctx, tx, requester)
	if err != nil {
		return err
	}
	return s.CreateRecordGrantTx(ctx, tx, CreateGrantInput{
		RecordType:  recordType,
		RecordID:    recordID,
		SubjectType: grantSubjectUser,
		SubjectID:   requester,
		Access:      access,
	})
}
