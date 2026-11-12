// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type InquiryAssignment struct {
	Reason           string
	RequestTypeField *string
}

// Assignments exposes handling facts only for task identities the reader may open.
func (s *InquiryService) Assignments(ctx context.Context, tasks []ids.ActivityID) (map[ids.UUID]InquiryAssignment, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return map[ids.UUID]InquiryAssignment{}, nil
	}
	if len(tasks) > 200 {
		return nil, apperrors.ErrInvalidArgument
	}
	if s.routing == nil {
		return nil, errors.New("automation: inquiry routing is not configured")
	}
	submissions, err := s.routing.TaskSubmissions(ctx, tasks)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(submissions))
	for _, key := range submissions {
		keys = append(keys, key)
	}
	plans, err := s.claims.inquiryPlans(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID]InquiryAssignment{}
	for id, key := range submissions {
		if plan, found := plans[key]; found {
			out[id] = InquiryAssignment{Reason: plan.Reason, RequestTypeField: plan.RequestTypeField}
		}
	}
	return out, nil
}
