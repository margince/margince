// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RegisterUTCTimes reads every timestamptz and timestamptz[] back in UTC, so a
// response never carries the process's own offset. A day entry's stored noon
// renders as T12:00:00Z, whose date is its date in any client's offset.
func RegisterUTCTimes(conn *pgx.Conn) {
	registerUTCTimes(conn.TypeMap())
}

func registerUTCTimes(m *pgtype.Map) {
	instant := &pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}}
	m.RegisterType(instant)
	m.RegisterType(&pgtype.Type{Name: "_timestamptz", OID: pgtype.TimestamptzArrayOID, Codec: &pgtype.ArrayCodec{ElementType: instant}})
}
