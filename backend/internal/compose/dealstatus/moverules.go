// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type moveRule string

const (
	rulePrepareMeeting  moveRule = "prepare_meeting"
	ruleMeetingFollowUp moveRule = "meeting_follow_up"
	ruleOpenTask        moveRule = "open_task"
	ruleRequestReminder moveRule = "request_reminder"
	ruleAcceptRequest   moveRule = "accept_request"
	ruleFarMeeting      moveRule = "future_meeting"
	ruleFirstOutreach   moveRule = "first_outreach"
	ruleNextStep        moveRule = "next_step"
	ruleMeetingRequest  moveRule = "meeting_request"
)

type moveRuleDefinition struct {
	action   string
	validate func(crmcontracts.DealStatusCardMove, crmcontracts.DealStatusCardMove) bool
}

var moveRules = map[moveRule]moveRuleDefinition{
	rulePrepareMeeting:  {ActionOpenMeetingBrief, sameCurrentBasis},
	ruleMeetingFollowUp: {ActionDraftEmail, sameCurrentBasis},
	ruleOpenTask:        {ActionOpenTask, sameCurrentBasis},
	ruleRequestReminder: {ActionDraftEmail, sameCurrentBasis},
	ruleAcceptRequest:   {ActionCreateTask, sameCurrentBasis},
	ruleFarMeeting:      {ActionOpenMeetingBrief, sameCurrentBasis},
	ruleFirstOutreach:   {ActionDraftEmail, sameCurrentBasis},
	ruleNextStep:        {ActionCreateTask, sameCurrentBasis},
	ruleMeetingRequest:  {ActionCreateTask, sameCurrentBasis},
}

func ruleMove(rule moveRule, reason string, args map[string]any, evidence ...crmcontracts.DealNextBestActionEvidence) crmcontracts.DealStatusCardMove {
	definition, registered := moveRules[rule]
	if !registered || definition.validate == nil {
		return crmcontracts.DealStatusCardMove{Action: ActionNone}
	}
	out := move(definition.action, reason, args, evidence...)
	out.Recommendation = &crmcontracts.RecommendationReceipt{Rule: string(rule)}
	return out
}

func sameCurrentBasis(stored, current crmcontracts.DealStatusCardMove) bool {
	a, b := stored.Recommendation, current.Recommendation
	return a != nil && b != nil && a.Rule == b.Rule && stored.Action == current.Action &&
		a.PolicyVersion == b.PolicyVersion && a.DealId == b.DealId &&
		a.BasisFingerprint == b.BasisFingerprint && a.BasisFingerprint != ""
}

// Receipt metadata cannot change the action a reader chose to set aside.
func semanticArguments(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		if key != "recommendation" {
			out[key] = value
		}
	}
	return out
}
