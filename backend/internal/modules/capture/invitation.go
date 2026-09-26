// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CalendarInvitationResolver joins provider echoes to already reserved activities.
type CalendarInvitationResolver func(context.Context, pgx.Tx, connector.NaturalKey, []byte) (ids.ActivityID, bool, error)

// WithCalendarInvitations binds the shared identity resolver before capture writes.
func (s *Sink) WithCalendarInvitations(resolve CalendarInvitationResolver) *Sink {
	clone := *s
	clone.resolveInvitation = resolve
	return &clone
}
