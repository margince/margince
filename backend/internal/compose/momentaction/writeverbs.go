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

// worklistVerbWrites answers, for every Worklist move, which refusal a reader
// without the write grant gets. Nil means the verb only reads. The deal card's
// moves are a subset of this vocabulary. gates/momentactionvocabulary_test.go
// holds the keys equal to the enum.
var worklistVerbWrites = map[crmcontracts.WorklistMoveAction]*langcopy.Phrase{
	crmcontracts.WorklistMoveActionDraftReply:       &blockedWords.Send,
	crmcontracts.WorklistMoveActionDraftEmail:       &blockedWords.Send,
	crmcontracts.WorklistMoveActionCreateTask:       &blockedWords.Task,
	crmcontracts.WorklistMoveActionOpenTask:         nil,
	crmcontracts.WorklistMoveActionOpenMeetingBrief: nil,
	crmcontracts.WorklistMoveActionReconnect:        nil,
	crmcontracts.WorklistMoveActionNone:             nil,
}

// suggestionVerbWrites is the same answer for the company brief's actions.
var suggestionVerbWrites = map[crmcontracts.Company360SuggestionActionKind]*langcopy.Phrase{
	crmcontracts.Company360SuggestionActionKindDraftReply: &blockedWords.Send,
	crmcontracts.Company360SuggestionActionKindAddTask:    &blockedWords.Task,
	crmcontracts.Company360SuggestionActionKindOpenDeal:   nil,
}

// MoveRefusal says why this reader may not take a Worklist or deal-card verb,
// in the reader's language, or nil when they may.
func MoveRefusal(ctx context.Context, action crmcontracts.WorklistMoveAction) *string {
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
func refusal[K comparable](ctx context.Context, verbs map[K]*langcopy.Phrase, verb K) *string {
	phrase, known := verbs[verb]
	if known && phrase == nil {
		return nil
	}
	if auth.AllowsWrite(ctx, "activity", principal.ActionCreate) {
		return nil
	}
	if phrase == nil {
		phrase = &blockedWords.Log
	}
	reason := langcopy.For(string(systemtext.Locale(ctx))).Say(*phrase)
	return &reason
}
