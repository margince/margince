// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"slices"
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
	return fmt.Sprintf(`(`+humanOrImportedTaskSQL(alias)+` OR `+HumanWroteActivityClause(alias)+`
 OR (`+pinner+`, %[1]s.id::text) IN (SELECT reader_id, row_id FROM `+auth.EffectiveWorklistPinsSQL(arg)+` adopted WHERE adopted.source='task')
 OR %[1]s.source_system = ANY($%[4]d)
 OR (%[1]s.source_system = $%[2]d AND `+source+`)
 OR (%[1]s.source_system = $%[3]d AND `+request+`)
 OR (%[1]s.source_system = ANY($%[5]d) AND `+question+`))`, alias, arg(provenance.CommitmentTaskSource), arg(provenance.EmailRequestSource),
		arg(taskSourcesOf(currentByItsWriter)), arg(taskSourcesOf(currentBySourceMail)))
}

// humanOrImportedTaskSQL names the tasks that stay current by their own
// lifecycle with no automatic family. One has no source_system: a human or an
// agent created it by hand. The other is in an importer's reserved namespace.
// Any other source_system waits for its family in taskSourceFamilies, or for a
// human to write or adopt the task.
func humanOrImportedTaskSQL(alias string) string {
	return fmt.Sprintf(`(coalesce(%[1]s.source_system, '') = '' OR starts_with(%[1]s.source_system, '%[2]s'))`,
		alias, provenance.ReservedSourceSystemPrefix)
}

// taskCurrency is how an automatic task stays current work.
type taskCurrency int

const (
	// currentByCommitment: its promise's admission is current (CurrentCommitmentClause).
	currentByCommitment taskCurrency = iota + 1
	// currentByRequest: its request is recent, unanswered and addressed to the assignee.
	currentByRequest
	// currentByItsWriter: the writer assigned it by its own rule and it lives
	// by the task's lifecycle.
	currentByItsWriter
	// currentBySourceMail: the mail it asks about is current (CurrentSourceMailClause).
	currentBySourceMail
)

// taskSourceFamilies is every source_system a native writer stamps on a task
// it creates with no human behind it, with how that task stays current.
//
// Held by: TestEveryAutomaticTaskWriterAndOpenTaskReaderIsDeclared (backend/gates/automatictaskwriters_test.go)
var taskSourceFamilies = map[string]taskCurrency{
	provenance.CommitmentTaskSource:     currentByCommitment,
	provenance.EmailRequestSource:       currentByRequest,
	provenance.NoActivityReminderSource: currentByItsWriter,
	provenance.CheckInCadenceSource:     currentByItsWriter,
	provenance.LeadSLATaskSource:        currentByItsWriter,
	provenance.LeadQualifyTaskSource:    currentByItsWriter,
	provenance.AssuranceTaskSource:      currentByItsWriter,
	provenance.OvernightReconcileSource: currentByItsWriter,
	provenance.WhichDealSource:          currentBySourceMail,
}

// TaskSourceFamilies lists the automatic task sources, for the census gate.
func TaskSourceFamilies() []string { return automaticTaskSources() }

func automaticTaskSources() []string {
	out := make([]string, 0, len(taskSourceFamilies))
	for source := range taskSourceFamilies {
		out = append(out, source)
	}
	slices.Sort(out)
	return out
}

func taskSourcesOf(currency taskCurrency) []string {
	var out []string
	for source, how := range taskSourceFamilies {
		if how == currency {
			out = append(out, source)
		}
	}
	slices.Sort(out)
	return out
}

// CurrentSourceMailClause is the test that the mail some automatic work is
// about is current. It is live, not in the future, and inside the waiting
// horizon. The mail's own time decides, never the capture's. source is a
// compile-time column reference.
//
// It reads metadata only, not body completeness. The work it keeps current is
// which deal a message is filed on, decided from the thread and the records.
// A body not read in full does not make that wrong. Work derived from the
// text reads provenance.MailBodyCompleteSQL.
func CurrentSourceMailClause(source string, asOf time.Time, arg func(any) int) string {
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	return `EXISTS (SELECT 1 FROM activity current_source WHERE current_source.id = ` + source + `
 AND current_source.archived_at IS NULL AND current_source.occurred_at <= ` + instant + `
 AND current_source.occurred_at >= ` + instant + ` - make_interval(days => ` + WaitingHorizonSQL(asOf, arg) + `))`
}
