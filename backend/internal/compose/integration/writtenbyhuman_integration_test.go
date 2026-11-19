// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The SQL rule every statement renders and the Go rule the read contract's
// automatic_kind comes from give one answer for every row shape. A forward to
// self is SQL's alone, so every row here is none.
func TestTheGoAndSQLWrittenByHumanRulesAgree(t *testing.T) {
	e := Setup(t)
	var rows []relstrength.Evidence
	for _, kind := range []string{"email", "call", "meeting", "message", "note"} {
		for _, origin := range []string{"human", "agent", "system_notice", "system_remediation"} {
			for _, method := range []string{"", "REQUEST", "REPLY", "CANCEL"} {
				for _, machine := range []bool{false, true} {
					for _, bulk := range []bool{false, true} {
						rows = append(rows, relstrength.Evidence{Kind: kind, Origin: origin, CalendarMethod: method, SenderIsMachine: machine, BulkMail: bulk})
					}
				}
			}
		}
	}
	for _, row := range rows {
		var method *string
		if row.CalendarMethod != "" {
			method = &row.CalendarMethod
		}
		inSQL := e.WsCount(t, `SELECT count(*)
			FROM (SELECT $1::text AS kind, $2::text AS origin, $3::text AS calendar_method, $4::boolean AS sender_is_machine,
			             $5::boolean AS bulk_mail_attested, false AS forwarded_to_self) a
			WHERE `+relstrength.WrittenByHumanSQL("a"),
			row.Kind, row.Origin, method, row.SenderIsMachine, row.BulkMail) == 1
		if inGo := row.Automatic() == ""; inGo != inSQL {
			t.Errorf("%+v: Go says a human wrote it = %v, SQL = %v", row, inGo, inSQL)
		}
	}
}
