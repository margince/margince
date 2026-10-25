// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

// A finding that says something is still owed is held to its thread's verdict
// (activities.SettledCitations). It is checked when written and when served,
// and the model's reading of the thread never overrules it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// claimsOwed is a finding kind that says the cited message is still owed.
func claimsOwed(kind crmcontracts.Company360SuggestionKind) bool {
	return kind == crmcontracts.Company360SuggestionKindCommitmentUnmet ||
		kind == crmcontracts.Company360SuggestionKindQuestionUnanswered
}

// standingRefusal says why a finding cannot rest on this message for this
// account, or "" when it can.
func standingRefusal(kind crmcontracts.Company360SuggestionKind, message MessageIn, partner bool) string {
	if message.Settled && claimsOwed(kind) {
		return "the message was already answered or kept in its thread, so nothing is owed on it"
	}
	if partner && kind == crmcontracts.Company360SuggestionKindNeedRaised {
		return "the account is a partner; its business is recorded in the partner programme, never as a deal to open"
	}
	return ""
}

// dropSettled removes the stored model findings whose cited message has since
// been settled in its thread. A second pass over its answer drops nothing.
func (s *Service) dropSettled(
	ctx context.Context, findings []crmcontracts.Company360Suggestion,
) ([]crmcontracts.Company360Suggestion, error) {
	var cited []ids.UUID
	for _, finding := range findings {
		if id, ok := owedCitation(finding); ok {
			cited = append(cited, id)
		}
	}
	if len(cited) == 0 {
		return findings, nil
	}
	var settled map[ids.UUID]bool
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var readErr error
		settled, readErr = activities.SettledCitations(ctx, tx, cited, s.now().UTC())
		return readErr
	})
	if err != nil {
		return nil, err
	}
	kept := make([]crmcontracts.Company360Suggestion, 0, len(findings))
	for _, finding := range findings {
		if id, ok := owedCitation(finding); ok && settled[id] {
			continue
		}
		kept = append(kept, finding)
	}
	return kept, nil
}

// owedCitation is the message a model finding claiming something is owed
// rests on. ground writes one activity citation per finding.
func owedCitation(finding crmcontracts.Company360Suggestion) (ids.UUID, bool) {
	if finding.WrittenBy == nil || *finding.WrittenBy != crmcontracts.WrittenByModel || !claimsOwed(finding.Kind) {
		return ids.UUID{}, false
	}
	for _, evidence := range finding.Evidence {
		if evidence.EntityType == crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
			return ids.UUID(evidence.EntityId), true
		}
	}
	return ids.UUID{}, false
}
