// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConnectionView supplies settings and health without exposing provider credentials.
type ConnectionView struct {
	SyncStalled           bool
	ConnectedAt           time.Time
	LastDeliveryAt        *time.Time
	SilenceDismissedSince *time.Time
	Concern               string
	ConcernSince          *time.Time
	ID                    ids.UUID
	Provider              string
	Status                string
	// DefaultSender marks the mailbox the member's new mail goes out from.
	DefaultSender  bool
	Cursor         []byte
	WatchExpiresAt *time.Time
	// A missing provider grant is unknown, not an empty scope.
	ProviderScopes []string
	// AccountLabel is display text, never an identity or routing key.
	AccountLabel         *string
	CalendarAccountLabel *string
	MailPosture          string
	ContextTag           *ContextTag
	// Attempts include failures; success stamps only completed pulls.
	LastSyncedAt   *time.Time
	LastSuccessAt  *time.Time
	LastErrorClass *string
	NextSyncDueAt  *time.Time
	FailingSince   *time.Time
	// Import progress does not attest a complete mail interval.
	Backfill *BackfillRun
	// HistoryNarrowing is nil unless earlier mail is still being narrowed.
	HistoryNarrowing *HistoryNarrowing
	narrowingHeld    *bool
	// A missing signature choice follows the workspace setting.
	SignatureEnrichEnabled *bool
	HistoryBaseline        *HistoryBaseline
	CanBackfill            bool
}
