// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CurrentTaskClause requires automatic evidence addressed to the assignee, or
// shared-mailbox mail the assignee holds the work of, or human adoption.
// alias is a compile-time activity alias, never request input.
func CurrentTaskClause(alias string, asOf time.Time, identities string, arg func(any) int) string {
	return scopedTaskClause(alias, asOf, identities, arg, true)
}

// CurrentTaskClauseToLineOnly is CurrentTaskClause whose reply reminders count
// only when the To line names their assignee. A reader that leaves out every
// task the product filed asks it: the shared-mailbox step decides none of them.
func CurrentTaskClauseToLineOnly(alias string, asOf time.Time, identities string, arg func(any) int) string {
	return scopedTaskClause(alias, asOf, identities, arg, false)
}

func scopedTaskClause(alias string, asOf time.Time, identities string, arg func(any) int, sharedMail bool) string {
	pinner := pgx.Identifier{alias, "assignee_id"}.Sanitize()
	alias = pgx.Identifier{alias}.Sanitize()
	source := provenance.CurrentCommitmentClause(alias+".source_activity_id", alias+".source_id", alias+".due_at", asOf, arg)
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	horizon := WaitingHorizonSQL(asOf, arg)
	held := ""
	if sharedMail {
		held = "\n OR " + SharedMailWorkOwnerSQL("request_source") + " = " + pinner
	}
	request := `EXISTS (SELECT 1 FROM activity request_source WHERE request_source.id = ` + alias + `.source_activity_id
 AND request_source.kind = 'email' AND request_source.direction = 'inbound' AND request_source.archived_at IS NULL
 AND ` + AutomaticRequestIntentClause("request_source") + `
 AND NOT ` + AnsweredSQL("request_source", instant) + `
 AND NOT EXISTS (SELECT 1 FROM activity successor WHERE successor.source_system = ` + alias + `.source_system
 AND successor.source_activity_id = ` + alias + `.source_activity_id AND successor.id <> ` + alias + `.id
 AND ` + HeldTaskClause("successor") + `)
 AND ` + occurredday.BegunSQL("request_source", instant) + `
 AND ` + occurredday.FromSQL("request_source", instant+` - make_interval(days => `+horizon+`)`) + ` AND (` + DirectMailRecipientSQL("request_source", MailSeatAddressesSQL(identities, pinner)) + held + `))`
	engagement := CurrentLeadEngagementClause(provenance.TaskSourceActivitySQL(alias), asOf, arg) + " AND " + latestLeadAdviceSQL(alias)
	question := CurrentSourceMailClause(alias+".source_activity_id", asOf, arg)
	return fmt.Sprintf(`(`+humanOrImportedTaskSQL(alias)+` OR `+HumanWroteActivityClause(alias)+`
 OR (`+pinner+`, %[1]s.id::text) IN (SELECT reader_id, row_id FROM `+auth.EffectiveWorklistPinsSQL(arg)+` adopted WHERE adopted.source='task')
 OR %[1]s.source_system = ANY($%[4]d)
 OR (%[1]s.source_system = $%[2]d AND `+source+`)
 OR (%[1]s.source_system = $%[3]d AND `+request+`)
 OR (%[1]s.source_system = ANY($%[5]d) AND `+question+`)
 OR (%[1]s.source_system = ANY($%[6]d) AND `+engagement+`))`, alias, arg(provenance.CommitmentTaskSource), arg(provenance.EmailRequestSource),
		arg(taskSourcesOf(currentByItsWriter)), arg(taskSourcesOf(currentBySourceMail)), arg(taskSourcesOf(currentByLeadEngagement)))
}

// humanOrImportedTaskSQL names the tasks that stay current by their own
// lifecycle with no automatic family. One has no source_system: a human or an
// agent created it by hand. One is in an importer's reserved namespace. The
// last is a task an extension unit filed for a named colleague. The capture
// door refuses an assignee who cannot hold work, so it is already addressed.
// Any other source_system waits for its family in taskSourceFamilies, or for a
// human to write or adopt the task.
func humanOrImportedTaskSQL(alias string) string {
	return fmt.Sprintf(`(coalesce(%[1]s.source_system, '') = ''
 OR starts_with(%[1]s.source_system, '%[2]s') OR starts_with(%[1]s.source_system, '%[3]s'))`,
		alias, provenance.ReservedSourceSystemPrefix, connector.ExtensionSourceSystemPrefix)
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
	currentByLeadEngagement
)

// Native task provenance shares its current-work and date policy here.
//
// Held by: TestEveryAutomaticTaskWriterAndOpenTaskReaderIsDeclared (backend/gates/automatictaskwriters_test.go)
type taskSourcePolicy struct {
	currency      taskCurrency
	undated       bool
	generatedDate bool
	// taskType is the work the source's tasks ask for, read by a task that
	// was not typed when it was made (TaskTypeSQL).
	taskType crmcontracts.TaskType
}

var taskSourceFamilies = map[string]taskSourcePolicy{
	// A follow-up the reader accepted from a suggestion is theirs to do even
	// without a date, so it shows with their tasks.
	provenance.AcceptedRecommendationSource: {currency: currentByItsWriter, undated: true},
	provenance.CommitmentTaskSource:         {currency: currentByCommitment, undated: true, taskType: crmcontracts.TaskTypeReachOut},
	provenance.EmailRequestSource:           {currency: currentByRequest, undated: true, taskType: crmcontracts.TaskTypeReply},
	provenance.NoActivityReminderSource:     {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReachOut},
	provenance.WebsiteInquirySource:         {currency: currentByItsWriter},
	provenance.CheckInCadenceSource:         {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReachOut},
	provenance.LeadSLATaskSource:            {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReachOut},
	provenance.LeadQualifyTaskSource:        {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReview},
	provenance.AssuranceTaskSource:          {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReview},
	provenance.AnalyticsDecisionSource:      {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReview},
	provenance.OvernightReconcileSource:     {currency: currentByItsWriter, taskType: crmcontracts.TaskTypeReview},
	provenance.LeadRepliedSource: {
		currency: currentByLeadEngagement, undated: true, generatedDate: true,
		taskType: crmcontracts.TaskTypeReview,
	},
	// A refused submission has no activity source; the owner decides its lifecycle.
	provenance.LeadForContactSource: {
		currency: currentByItsWriter, undated: true, generatedDate: true,
		taskType: crmcontracts.TaskTypeReview,
	},
	provenance.WhichDealSource: {currency: currentBySourceMail, taskType: crmcontracts.TaskTypeDecide},
}

// TaskSourceFamilies lists native task sources for the census gate.
func TaskSourceFamilies() []string { return automaticTaskSources() }

// TaskTypeSQL is the type a task reads: the one it was given, else its
// source's. Without either, a task a human or an agent made reads reach_out, and
// the rest read do. alias is a compile-time activity alias, never request input.
func TaskTypeSQL(alias string) string {
	alias = pgx.Identifier{alias}.Sanitize()
	sources := make([]string, 0, len(taskSourceFamilies))
	for _, source := range automaticTaskSources() {
		if taskType := taskSourceFamilies[source].taskType; taskType != "" {
			sources = append(sources, "WHEN '"+source+"' THEN '"+string(taskType)+"'")
		}
	}
	return "(CASE WHEN " + alias + ".kind <> 'task' THEN NULL ELSE coalesce(" + alias + ".task_type, CASE " +
		alias + ".source_system " + strings.Join(sources, " ") + " END, CASE WHEN " +
		relstrength.OriginIsEngagementSQL(alias) + " THEN '" + string(crmcontracts.TaskTypeReachOut) +
		"' ELSE '" + string(crmcontracts.TaskTypeDo) + "' END) END)"
}

// ReplyChannelSQL is the kind of the activity a reply task answers, so the
// answer goes back the way the message came in. Null for any other task, and
// for a held source, whose kind is not this reader's to see.
func ReplyChannelSQL(alias string) string {
	taskType := TaskTypeSQL(alias)
	alias = pgx.Identifier{alias}.Sanitize()
	return "(CASE WHEN " + taskType + " = '" + string(crmcontracts.TaskTypeReply) +
		"' THEN (SELECT s.kind FROM activity s WHERE s.id = " + alias + ".source_activity_id AND s.restricted_at IS NULL AND s.kind IN ('" +
		string(crmcontracts.ActivityKindEmail) + "', '" + string(crmcontracts.ActivityKindCall) + "', '" +
		string(crmcontracts.ActivityKindMessage) + "')) END)"
}

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
		if how.currency == currency {
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
 AND ` + SourceWithinHorizonSQL("current_source", asOf, arg) + `
 AND ` + occurredday.BegunSQL("current_source", instant) + `)`
}

// SourceWithinHorizonSQL admits a live row whose own time falls inside the
// waiting horizon. alias is a compile-time literal.
func SourceWithinHorizonSQL(alias string, asOf time.Time, arg func(any) int) string {
	return SourceWithinHorizonAtSQL(alias, fmt.Sprintf("$%d::timestamptz", arg(asOf)), asOf, arg)
}

// SourceWithinHorizonAtSQL is SourceWithinHorizonSQL judged at the instant
// expression at, such as the row's own capture time. asOf picks the horizon
// measurement in force; alias and at are compile-time literals.
func SourceWithinHorizonAtSQL(alias, at string, asOf time.Time, arg func(any) int) string {
	return alias + `.archived_at IS NULL AND ` + occurredday.FromSQL(alias, at+` - make_interval(days => `+WaitingHorizonSQL(asOf, arg)+`)`)
}

// UndatedTaskSources names current work that needs no invented deadline.
func UndatedTaskSources() []string {
	return taskSourcesWith(func(p taskSourcePolicy) bool { return p.undated })
}

func taskSourcesWith(keep func(taskSourcePolicy) bool) []string {
	var out []string
	for source, policy := range taskSourceFamilies {
		if keep(policy) {
			out = append(out, source)
		}
	}
	slices.Sort(out)
	return out
}

// TaskRepairPolicy derives repair scope from the same current-work policy as reads.
func TaskRepairPolicy(source string) (supported, requiresSource bool) {
	policy, known := taskSourceFamilies[source]
	return known && (policy.currency == currentBySourceMail || policy.currency == currentByLeadEngagement || policy.generatedDate),
		known && (policy.currency == currentBySourceMail || policy.currency == currentByLeadEngagement)
}
