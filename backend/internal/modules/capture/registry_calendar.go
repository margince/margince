// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
	if write && !CalendarWriteGranted(provider, scopes) {
		return nil, nil, connector.ErrAuthRejected
	}
	c, err := r.connector(provider)
	if err != nil {
		return nil, nil, err
	}
	c = connector.RotatingSyncer(c, &rotationSink{registry: r, connectionID: id, generation: generation, readRef: ref, log: slog.Default()})
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

var calendarWriteScopes = map[string][]string{
	"gcal": {
		"https://www.googleapis.com/auth/calendar.events.owned",
		"https://www.googleapis.com/auth/calendar.events",
		"https://www.googleapis.com/auth/calendar",
	},
	"graphcal": {"Calendars.ReadWrite"},
}

// calendarProviderNames are the providers that connect a calendar: each one
// names its write scopes above.
func calendarProviderNames() []string {
	return slices.Sorted(maps.Keys(calendarWriteScopes))
}

// CalendarProviders are the providers a seat may connect a calendar through.
func CalendarProviders() []string { return calendarProviderNames() }

// CalendarWriteGranted says whether a connection's granted scopes let it write
// events on provider's calendar. It is the one read-only detector.
func CalendarWriteGranted(provider string, scopes []string) bool {
	return scopeGranted(calendarWriteScopes, provider, scopes)
}

// scopeGranted says whether scopes hold any grant the table lists for
// provider. Every capability decided by an OAuth grant reads it.
func scopeGranted(grants map[string][]string, provider string, scopes []string) bool {
	for _, grant := range grants[provider] {
		if slices.Contains(scopes, grant) {
			return true
		}
	}
	return false
}
