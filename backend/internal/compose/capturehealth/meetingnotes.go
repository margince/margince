// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturehealth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// MeetingNotesAccess shares capture policy with activity readers.
type MeetingNotesAccess struct{}

// ForReaderTx evaluates access in the activity reader's transaction.
func (MeetingNotesAccess) ForReaderTx(ctx context.Context, tx pgx.Tx, now time.Time) (activities.MeetingNotesAccess, error) {
	access, err := capture.MeetingNotesAccessTx(ctx, tx, now)
	return activities.MeetingNotesAccess{Concern: access.Concern, Domains: access.Domains, Since: access.Since}, err
}
