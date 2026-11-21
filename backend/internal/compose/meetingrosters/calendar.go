// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package meetingrosters holds the data tasks that repair a meeting's guest
// rows from the calendar event stored for it.
package meetingrosters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The calendar connectors, as source_system and captured_by spell them.
const (
	GoogleCalendar = "gcal"
	GraphCalendar  = "graphcal"
)

// runEvidence keys the data task run in an audit row's evidence.
const runEvidence = "data_task_run"

// storedCalendar is the address of the calendar a stored event was read from,
// or "" when the event proves none. An event no parser reads proves none. Only
// a failed read of the connected calendars is an error.
func storedCalendar(ctx context.Context, tx pgx.Tx, provider string, raw []byte) (string, error) {
	if provider == GoogleCalendar {
		calendar, err := gcal.SelfAddress(raw)
		if err != nil {
			return "", nil //nolint:nilerr // an unreadable original names no calendar
		}
		return calendar, nil
	}
	connected, err := database.Remember(tx, "meetingrosters.calendars:"+provider, func() ([]string, error) {
		rows, err := tx.Query(ctx, `SELECT account_label FROM capture_connection
			 WHERE provider = $1 AND coalesce(account_label, '') <> ''`, provider)
		if err != nil {
			return nil, fmt.Errorf("compose: reading the connected calendars: %w", err)
		}
		return pgx.CollectRows(rows, pgx.RowTo[string])
	})
	if err != nil {
		return "", err
	}
	calendar, err := graphcal.OwnCalendar(raw, connected)
	if err != nil {
		return "", nil //nolint:nilerr // an unreadable original names no calendar
	}
	return calendar, nil
}

// recordOf reads a stored event the way live capture does for its provider.
func recordOf(provider string, raw []byte, calendar string) (connector.NormalizedRecord, error) {
	if provider == GoogleCalendar {
		return gcal.RecordOf(raw, calendar)
	}
	return graphcal.RecordOf(raw, calendar)
}

// StampingSeat answers the seat whose calendar stamps the live meeting, and
// only when that calendar is the one the stored event came from.
func StampingSeat(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, provider, calendar string) (ids.UUID, bool, error) {
	var seat *ids.UUID
	var label string
	err := tx.QueryRow(ctx, `
		SELECT `+auth.ConnectorSeatSQL("a")+`::uuid, coalesce(c.account_label, '')
		  FROM activity a
		  LEFT JOIN capture_connection c
		    ON c.provider = $2 AND c.user_id::text = `+auth.ConnectorSeatSQL("a")+`
		 WHERE a.id = $1 AND a.kind = 'meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND split_part(a.captured_by, ':', 2) = $2
		 LIMIT 1`, meeting, provider).Scan(&seat, &label)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("compose: reading the calendar that stamps a meeting: %w", err)
	}
	if seat == nil || calendar == "" || !strings.EqualFold(strings.TrimSpace(label), calendar) {
		return ids.Nil, false, nil
	}
	return *seat, true, nil
}
