// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CurrentTaskClause requires automatic evidence addressed to the assignee or human adoption.
// alias is a compile-time activity alias, never request input.
func CurrentTaskClause(alias string, asOf time.Time, identities string, arg func(any) int) string {
	pinner := pgx.Identifier{alias, "assignee_id"}.Sanitize()
	alias = pgx.Identifier{alias}.Sanitize()
	source := provenance.CurrentCommitmentClause(alias+".source_activity_id", alias+".source_id", alias+".due_at", asOf, arg)
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	horizon := WaitingHorizonSQL(asOf, arg)
	request := `EXISTS (SELECT 1 FROM activity request_source WHERE request_source.id = ` + alias + `.source_activity_id
 AND request_source.kind = 'email' AND request_source.direction = 'inbound' AND request_source.archived_at IS NULL
 AND ` + AutomaticRequestIntentClause("request_source") + `
 AND NOT EXISTS (SELECT 1 FROM activity successor WHERE successor.source_system = ` + alias + `.source_system
 AND successor.source_activity_id = ` + alias + `.source_activity_id AND successor.id <> ` + alias + `.id
 AND ` + HeldTaskClause("successor") + `)
 AND request_source.occurred_at <= ` + instant + `
 AND request_source.occurred_at >= ` + instant + ` - make_interval(days => ` + horizon + `) AND ` + DirectMailRecipientSQL("request_source", MailSeatAddressesSQL(identities, pinner)) + `)`
	question := CurrentSourceMailClause(alias+".source_activity_id", asOf, arg)
	return fmt.Sprintf(`(coalesce(%[1]s.source_system, '') NOT IN ($%[2]d, $%[3]d, $%[4]d) OR `+HumanWroteActivityClause(alias)+`
 OR (`+pinner+`, %[1]s.id::text) IN (SELECT reader_id, row_id FROM `+auth.EffectiveWorklistPinsSQL(arg)+` adopted WHERE adopted.source='task')
 OR (%[1]s.source_system = $%[2]d AND `+source+`)
 OR (%[1]s.source_system = $%[3]d AND `+request+`)
 OR (%[1]s.source_system = $%[4]d AND `+question+`))`, alias, arg(provenance.CommitmentTaskSource), arg(provenance.EmailRequestSource),
		arg(provenance.WhichDealSource))
}

// CurrentSourceMailClause is the test that the mail a piece of automatic work
// is about is still current: live, not in the future, and inside the waiting
// horizon. An old mail captured today is not current, because its own time
// decides, never the capture's. source is a compile-time column reference.
func CurrentSourceMailClause(source string, asOf time.Time, arg func(any) int) string {
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	return `EXISTS (SELECT 1 FROM activity current_source WHERE current_source.id = ` + source + `
 AND current_source.archived_at IS NULL AND current_source.occurred_at <= ` + instant + `
 AND current_source.occurred_at >= ` + instant + ` - make_interval(days => ` + WaitingHorizonSQL(asOf, arg) + `))`
}
