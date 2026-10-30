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

// inputChanges decides, for every event on the deal stream and every forecast
// event, whether it makes the last reading out of date.
// TestEveryDealAndForecastEventHasAnInputDecision fails on a new type that is
// missing here, so a new event is a decision and never a silent gap.
//
// A capture connection changing health is not here because it emits no event:
// the next run grades the mailbox afresh, and until then the run's own source
// coverage says how far it read.
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
		ScopeNone, "who may read a room is not a rule input"},
	"deal_room.participant_revoked": {
		ScopeNone, "who may read a room is not a rule input"},
	"deal_room.participant_credential_reissued": {
		ScopeNone, "who may read a room is not a rule input"},
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

	"deal_suggestion.created":    {ScopeNone, "a suggestion is not a deal until it is accepted"},
	"deal_suggestion.accepted":   {ScopeNone, "an accepted suggestion announces its deal as deal.created"},
	"deal_suggestion.dismissed":  {ScopeNone, "a suggestion is not a deal until it is accepted"},
	"deal_suggestion.superseded": {ScopeNone, "a suggestion is not a deal until it is accepted"},
	"legal_review.requested":     {ScopeNone, "a legal review is not a rule input"},
	"legal_review.decided":       {ScopeNone, "a legal review is not a rule input"},
	"legal_review.withdrawn":     {ScopeNone, "a legal review is not a rule input"},

	"forecast.created": {ScopeWorkspace, "a call submitted after the reading was not judged against it"},
	"forecast.exception_resolved": {
		ScopeNone, "an answer changes the finding itself, and the list shows that at once"},
	"forecast.assurance_created": {ScopeNone, "this is the reading itself"},
	"forecast.snapshot_created":  {ScopeNone, "a snapshot records the forecast and changes no input"},
	"forecast.share_issued":      {ScopeNone, "who may read the forecast is not a rule input"},
	"forecast.share_revoked":     {ScopeNone, "who may read the forecast is not a rule input"},
}

// InputChangeOf answers the decision for one event type. ok is false for a
// type outside the deal stream and the forecast family.
func InputChangeOf(eventType string) (InputChange, bool) {
	change, ok := inputChanges[eventType]
	return change, ok
}

// DecidedInputEvents is every event type with a decision, sorted.
func DecidedInputEvents() []string {
	out := make([]string, 0, len(inputChanges))
	for eventType := range inputChanges {
		out = append(out, eventType)
	}
	sort.Strings(out)
	return out
}
