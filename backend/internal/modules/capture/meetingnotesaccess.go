// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const notesOwnerArg = "owner"

// NotesAccess is the reader's notes grant, pairing and trusted organizer domains.
type NotesAccess struct {
	Concern string
	Domains []string
	Since   time.Time
}

// MeetingNotesAccessTx evaluates grants and account pairing under the reader's transaction.
func MeetingNotesAccessTx(ctx context.Context, tx pgx.Tx, now time.Time) (NotesAccess, error) {
	result := NotesAccess{Concern: "connect", Since: now.Add(-meetingNotesWindow)}
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return result, nil
	}
	trusted, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Domains = trusted.Domains()
	var provider string
	var scopes []string
	var notes, calendar *string
	err = tx.QueryRow(ctx, `SELECT c.provider, c.provider_scopes, c.account_label, `+notesCalendarAccountSQL+`
 FROM capture_connection c WHERE c.user_id = @owner AND c.provider = 'gdocs'
 AND c.status IN ('connected','error') AND c.archived_at IS NULL`, pgx.NamedArgs{notesOwnerArg: actor.UserID}).Scan(&provider, &scopes, &notes, &calendar)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("capture: reading the member's meeting-notes access: %w", err)
	}
	result.Concern = MeetingNotesConcern(provider, scopes, notes, calendar)
	return result, nil
}
