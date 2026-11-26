// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeSubjectBriefCache destroys every reader's cached relationship brief for
// a subject the caller has just anonymized. The brief holds what the model was
// shown: claims from the subject's conversations and a line on each message.
//
// The erasure leaves the contact row in place, so no ON DELETE CASCADE fires.
// The row is deleted and not emptied, because the next read rebuilds it.
func purgeSubjectBriefCache(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	if err := generatedsummary.ForgetContacts(ctx, tx, subject); err != nil {
		return fmt.Errorf("privacy: destroying the subject's cached relationship brief: %w", err)
	}
	return nil
}
