// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// worklistVerbWrites answers, for every Worklist move, whether it writes an
// activity. The deal card's moves are a subset of this vocabulary.
// gates/momentactionvocabulary_test.go holds the keys equal to the enum.
var worklistVerbWrites = map[crmcontracts.WorklistMoveAction]bool{
	crmcontracts.WorklistMoveActionDraftReply:       true,
	crmcontracts.WorklistMoveActionDraftEmail:       true,
	crmcontracts.WorklistMoveActionCreateTask:       true,
	crmcontracts.WorklistMoveActionOpenTask:         false,
	crmcontracts.WorklistMoveActionOpenMeetingBrief: false,
	crmcontracts.WorklistMoveActionReconnect:        false,
	crmcontracts.WorklistMoveActionBringInManager:   false,
	crmcontracts.WorklistMoveActionNone:             false,
}

// suggestionVerbWrites is the same answer for the company brief's actions.
var suggestionVerbWrites = map[crmcontracts.Company360SuggestionActionKind]bool{
	crmcontracts.Company360SuggestionActionKindDraftReply: true,
	crmcontracts.Company360SuggestionActionKindAddTask:    true,
	crmcontracts.Company360SuggestionActionKindOpenDeal:   false,
}

// MoveRefusal says why this reader may not take a Worklist or deal-card verb,
// in the reader's language, or nil when they may.
func MoveRefusal(ctx context.Context, action crmcontracts.WorklistMoveAction) *string {
	// Asking the manager changes the deal, not an activity, so it asks the deal grant.
	if action == crmcontracts.WorklistMoveActionBringInManager {
		if auth.AllowsWrite(ctx, "deal", principal.ActionUpdate) {
			return nil
		}
		reason := grantRefusal(ctx, langcopy.For(string(systemtext.Locale(ctx))), blockedWords.Deal)
		return &reason
	}
	return refusal(ctx, worklistVerbWrites, action)
}

// MarkRefusedSuggestions blocks each company brief action this reader may not
// take. The company page and the account scan both serve these rows.
func MarkRefusedSuggestions(ctx context.Context, suggestions []crmcontracts.Company360Suggestion) {
	for i := range suggestions {
		if action := suggestions[i].Action; action != nil {
			action.BlockedReason = refusal(ctx, suggestionVerbWrites, action.Kind)
		}
	}
}

// refusal fails closed. A verb missing from the map is refused like a write,
// so a new verb is not offered live before someone classifies it.
func refusal[K comparable](ctx context.Context, verbs map[K]bool, verb K) *string {
	if writes, known := verbs[verb]; known && !writes {
		return nil
	}
	if auth.AllowsWrite(ctx, "activity", principal.ActionCreate) {
		return nil
	}
	reason := langcopy.For(string(systemtext.Locale(ctx))).Say(blockedWords.RoleReadsOnly)
	return &reason
}
