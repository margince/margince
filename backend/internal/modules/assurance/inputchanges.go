// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import "sort"

// InputScope is how far one kind of change reaches into the last reading.
type InputScope string

const (
	// ScopeDeal is a change to one deal's inputs. That deal's findings, and the
	// run that judged it, are out of date.
	ScopeDeal InputScope = "deal"
	// ScopeSignal is a change to what one buying signal's finding read: the
	// signal, its company, or a suggestion that covers it.
	ScopeSignal InputScope = "signal"
	// ScopeDealActivity is a change to an activity. It reaches each open deal
	// the activity is linked to whose viability fingerprint moved since the
	// run stored it. An edit the check does not read marks nothing.
	ScopeDealActivity InputScope = "deal_activity"
	// ScopeWorkspace is a change no one deal carries: a stage every deal in it
	// is judged against, or a forecast call made on the reading. Every finding
	// and the run are out of date.
	ScopeWorkspace InputScope = "workspace"
	// ScopeNone is a change no rule reads.
	ScopeNone InputScope = "none"
)

// InputChange is the decision for one event type, with the reason for it.
type InputChange struct {
	Scope InputScope
	Why   string
}

// inputChanges decides whether an event makes the last reading out of date.
// It covers the deal stream, the forecast family and every table the check
// reads. TestEveryDealAndForecastEventHasAnInputDecision and compose's
// TestEveryTableTheCheckReadsHasItsEventsDecided fail on a missing type.
//
// A capture connection changing health emits no event, so it is not here. The
// next run grades the mailbox afresh, and until then its coverage says how far
// it read.
//
//nolint:goconst // each row states its own reason; two events sharing one wording are still two decisions
var inputChanges = map[string]InputChange{
	"deal.created":       {ScopeDeal, "a new open deal was never judged"},
	"deal.updated":       {ScopeDeal, "amount, close date, category and wait are rule inputs"},
	"deal.stage_changed": {ScopeDeal, "the stage decides the exit criteria and a win or loss ends the check"},
	"deal.owner_changed": {ScopeDeal, "a finding belongs to the deal's owner"},
	"deal.archived":      {ScopeDeal, "an archived deal leaves the checked set"},
	"deal.restored":      {ScopeDeal, "a restored deal comes back unjudged"},
	"deal.merged":        {ScopeDeal, "the surviving deal now carries the other one's record"},
	"deal.erased":        {ScopeDeal, "an erased deal leaves the checked set"},

	"offer.created":    {ScopeDeal, "the offer total is compared with the deal amount"},
	"offer.sent":       {ScopeDeal, "the offer total is compared with the deal amount"},
	"offer.accepted":   {ScopeDeal, "the offer total is compared with the deal amount"},
	"offer.rejected":   {ScopeDeal, "the offer total is compared with the deal amount"},
	"offer.superseded": {ScopeDeal, "the offer total is compared with the deal amount"},

	"contract.created":        {ScopeDeal, "the contract value is compared with the deal amount"},
	"contract.updated":        {ScopeDeal, "the contract value is compared with the deal amount"},
	"contract.status_changed": {ScopeDeal, "only an active contract's value is compared"},
	"contract.archived":       {ScopeDeal, "an archived contract is no longer compared"},

	"deal_room.decision_recorded": {ScopeDeal, "a buyer's decision can settle an exit criterion"},
	"deal_room.opened":            {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.updated":           {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.paused":            {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.resumed":           {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.closed":            {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.archived":          {ScopeNone, "a room's lifecycle is not a rule input"},
	"deal_room.participant_invited": {
		ScopeNone, "who may read a room is not a rule input",
	},
	"deal_room.participant_revoked": {
		ScopeNone, "who may read a room is not a rule input",
	},
	"deal_room.participant_credential_reissued": {
		ScopeNone, "who may read a room is not a rule input",
	},
	"deal_room.comment_posted":  {ScopeNone, "the room's conversation is not a rule input"},
	"deal_room.thread_resolved": {ScopeNone, "the room's conversation is not a rule input"},

	"stage.created":     {ScopeWorkspace, "a stage's name and exit criteria are rule inputs"},
	"stage.updated":     {ScopeWorkspace, "a stage's name and exit criteria are rule inputs"},
	"stage.archived":    {ScopeWorkspace, "a stage's name and exit criteria are rule inputs"},
	"pipeline.archived": {ScopeWorkspace, "the deals of an archived pipeline leave the checked set"},
	"pipeline.created":  {ScopeNone, "a new pipeline holds no deal yet"},
	"pipeline.updated":  {ScopeNone, "a pipeline's own fields are not a rule input; its stages announce theirs"},

	"project.created":       {ScopeNone, "projects are not a rule input"},
	"project.updated":       {ScopeNone, "projects are not a rule input"},
	"project.phase_changed": {ScopeNone, "projects are not a rule input"},
	"project.archived":      {ScopeNone, "projects are not a rule input"},
	"commission.accrued":    {ScopeNone, "commission follows a won deal, which the check no longer reads"},
	"commission.decided":    {ScopeNone, "commission follows a won deal, which the check no longer reads"},

	"deal_suggestion.created":    {ScopeSignal, "an open suggestion citing a signal covers it"},
	"deal_suggestion.accepted":   {ScopeSignal, "a suggestion that is no longer open stops covering its signals"},
	"deal_suggestion.dismissed":  {ScopeSignal, "a suggestion that is no longer open stops covering its signals"},
	"deal_suggestion.superseded": {ScopeSignal, "a suggestion that is no longer open stops covering its signals"},
	"legal_review.requested":     {ScopeNone, "a legal review is not a rule input"},
	"legal_review.decided":       {ScopeNone, "a legal review is not a rule input"},
	"legal_review.withdrawn":     {ScopeNone, "a legal review is not a rule input"},

	"activity.captured": {ScopeDealActivity, "a deal's activities decide its viability and next step"},
	"activity.updated":  {ScopeDealActivity, "a deal's activities decide its viability and next step"},
	"activity.archived": {ScopeDealActivity, "an archived activity no longer counts for its deal"},
	"activity.disposition_recorded": {
		ScopeDealActivity, "a reply's verdict decides whether the customer refused",
	},
	"activity.marked_handled":     {ScopeDealActivity, "the fingerprint decides whether the deal's reading moved"},
	"activity.readiness_recorded": {ScopeDealActivity, "the fingerprint decides whether the deal's reading moved"},

	"signal.detected": {ScopeSignal, "a new buying signal was never judged"},
	"signal.updated":  {ScopeSignal, "the signal's evidence, company and status are rule inputs"},
	"signal.resolved": {ScopeSignal, "a resolved signal leaves the checked set"},

	"company.created":  {ScopeNone, "no signal is resolved to a company that did not exist"},
	"company.updated":  {ScopeSignal, "the company's owner and anchor flag decide its signals' findings"},
	"company.archived": {ScopeSignal, "an archived company's signals leave the checked set"},
	"company.restored": {ScopeSignal, "a restored company's signals come back unjudged"},
	"company.merged":   {ScopeSignal, "the surviving company now carries the other one's signals"},
	"company.erased":   {ScopeSignal, "an erased company's signals leave the checked set"},

	"forecast.created": {ScopeWorkspace, "a call submitted after the reading was not judged against it"},
	"forecast.exception_resolved": {
		ScopeNone, "an answer changes the finding itself, and the list shows that at once",
	},
	"forecast.assurance_created": {ScopeNone, "this is the reading itself"},
	"forecast.snapshot_created":  {ScopeNone, "a snapshot records the forecast and changes no input"},
	"forecast.share_issued":      {ScopeNone, "who may read the forecast is not a rule input"},
	"forecast.share_revoked":     {ScopeNone, "who may read the forecast is not a rule input"},
}

// InputChangeOf answers the decision for one event type. ok is false for a
// type the check does not read.
func InputChangeOf(eventType string) (InputChange, bool) {
	change, ok := inputChanges[eventType]
	return change, ok
}

// DecidedInputEvents lists the event types with a decision, sorted.
func DecidedInputEvents() []string {
	out := make([]string, 0, len(inputChanges))
	for eventType := range inputChanges {
		out = append(out, eventType)
	}
	sort.Strings(out)
	return out
}
