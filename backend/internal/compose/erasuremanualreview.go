// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/privacyreview"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (p capturedTextPurger) FlagSubjectReviewTx(ctx context.Context, tx pgx.Tx, subject ids.ContactID, sources []privacy.ErasureReviewSource) error {
	_, err := privacyreview.FlagTx(ctx, tx, notices.NewStore(p.db), subject.String(), sources)
	return err
}
