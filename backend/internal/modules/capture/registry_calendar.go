// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CalendarFor uses the host's live credential, including the existing rotation
// fence. The caller must first authorize access to that host's schedule.
//
//nolint:ireturn // optional connector capability
func (r *Registry) CalendarFor(ctx context.Context, host ids.UserID, provider string, write bool) (connector.CalendarScheduler, connector.Auth, error) {
	var id ids.UUID
	var ref *string
	var legacy []byte
	var scopes []string
	var generation int
	args := []any{host, provider}
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, credential_ref, auth, provider_scopes, generation FROM capture_connection`+sendableConnection,
			args...).Scan(&id, &ref, &legacy, &scopes, &generation)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNoConnection
	}
	if err != nil {
		return nil, nil, err
	}
	if write && !calendarWriteGranted(provider, scopes) {
		return nil, nil, connector.ErrAuthRejected
	}
	c, err := r.connector(provider)
	if err != nil {
		return nil, nil, err
	}
	c = connector.RotatingSyncer(c, rotationSink{registry: r, connectionID: id, generation: generation, readRef: ref, log: slog.Default()})
	scheduler, ok := c.(connector.CalendarScheduler)
	if !ok {
		return nil, nil, fmt.Errorf("calendar: scheduling unavailable: %w", ErrConnectorNotConfigured)
	}
	credential, err := r.resolveCredential(ctx, ref, legacy)
	if err != nil {
		return nil, nil, err
	}
	return scheduler, credential, nil
}

func calendarWriteGranted(provider string, scopes []string) bool {
	switch provider {
	case "gcal":
		return slices.Contains(scopes, "https://www.googleapis.com/auth/calendar.events.owned") ||
			slices.Contains(scopes, "https://www.googleapis.com/auth/calendar.events") || slices.Contains(scopes, "https://www.googleapis.com/auth/calendar")
	case "graphcal":
		return slices.Contains(scopes, "Calendars.ReadWrite")
	default:
		return false
	}
}
