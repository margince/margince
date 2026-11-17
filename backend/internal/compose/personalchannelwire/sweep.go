// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannelwire

import (
	"context"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Sweep runs one clean-up pass for the installation ctx is bound to.
func Sweep(ctx context.Context, db *database.DB, now time.Time, log *slog.Logger) error {
	swept, expired, err := personalchannels.Sweep(ctx, db, Settler{}, now)
	if swept+expired > 0 {
		log.InfoContext(ctx, "personal channels: swept", "files_deleted", swept, "replies_expired", expired)
	}
	return err
}
