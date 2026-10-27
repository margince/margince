// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	basisCurrentRequest = "current_request"
	basisRequestReview  = "request_review"
)

func requestForMove(f facts) (crmcontracts.Activity, bool, bool) {
	if current, ok := unansweredInbound(f); ok {
		return current, true, true
	}
	for _, candidate := range f.requests {
		if candidate.Kind == crmcontracts.ActivityKindEmail && !withheld(candidate) {
			return candidate, false, true
		}
	}
	return crmcontracts.Activity{}, false, false
}

func requestBasis(f facts, source crmcontracts.Activity, kind string) *crmcontracts.WorkEvidenceBasis {
	return &crmcontracts.WorkEvidenceBasis{
		Kind: kind, Scope: linkDeal, SourceActivityId: source.Id,
		MailCoverage: f.mailCoverage,
		SourceAt:     source.OccurredAt, SourcePrecision: source.OccurredPrecision,
		SourceVersion: source.Version, EvaluatedAt: f.now, PolicyVersion: ProjectionVersion,
	}
}

func requestMoveBasis(f facts, mv crmcontracts.DealStatusCardMove) *crmcontracts.WorkEvidenceBasis {
	source, current, ok := requestForMove(f)
	if !ok {
		return nil
	}
	operand, ok := operandActivity(mv)
	if !ok || operand != ids.UUID(source.Id) {
		return nil
	}
	kind := basisRequestReview
	if current {
		kind = basisCurrentRequest
	}
	return requestBasis(f, source, kind)
}

func factActivities(f facts) []crmcontracts.Activity {
	rows := make([]crmcontracts.Activity, 0, len(f.timeline)+len(f.requests)+len(f.currentRequests))
	seen := map[ids.UUID]bool{}
	for _, group := range [][]crmcontracts.Activity{f.currentRequests, f.requests, f.timeline} {
		for _, row := range group {
			id := ids.UUID(row.Id)
			if !seen[id] {
				rows = append(rows, row)
				seen[id] = true
			}
		}
	}
	return rows
}

func requestStates(f facts) map[string]string {
	states := make(map[string]string, len(f.requests)+len(f.currentRequests))
	for _, row := range f.requests {
		if !withheld(row) {
			states[row.Id.String()] = basisRequestReview
		}
	}
	for _, row := range f.currentRequests {
		if !withheld(row) {
			states[row.Id.String()] = basisCurrentRequest
		}
	}
	return states
}

func citesReviewRequest(citations []string, states map[string]string) bool {
	for _, id := range citations {
		if states[id] == basisRequestReview {
			return true
		}
	}
	return false
}
