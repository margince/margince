// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConnectionView supplies settings and health without exposing provider credentials.
type ConnectionView struct {
	ID             ids.UUID
	Provider       string
	Status         string
	Cursor         []byte
	WatchExpiresAt *time.Time
	// A missing provider grant is unknown, not an empty scope.
	ProviderScopes []string
	// AccountLabel is display text, never an identity or routing key.
	AccountLabel *string
	MailPosture  string
	ContextTag   *ContextTag
	// Attempts include failures; success stamps only completed pulls.
	LastSyncedAt   *time.Time
	LastSuccessAt  *time.Time
	LastErrorClass *string
	NextSyncDueAt  *time.Time
	FailingSince   *time.Time
	// Import progress does not attest a complete mail interval.
	Backfill *BackfillRun
	// A missing signature choice follows the workspace setting.
	SignatureEnrichEnabled *bool
	HistoryBaseline        *HistoryBaseline
	CanBackfill            bool
}
