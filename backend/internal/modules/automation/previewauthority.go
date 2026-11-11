// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// WithPreviewAuthority uses the same live authorizer grants as scheduled execution.
func (s *AutomationStore) WithPreviewAuthority(authority authz.Resolver) *AutomationStore {
	s.previewAuthority = authority
	return s
}

func (s *AutomationStore) previewReaders(ctx context.Context, stored Automation, in AutomationPreviewInput) ([]principal.Principal, error) {
	key := stored.Key
	if in.Key != nil {
		key = *in.Key
	}
	if !scheduledPopulationRule(key) || stored.OwnerID == nil {
		return nil, nil
	}
	caller, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	if caller.UserID == stored.OwnerID.UUID {
		return nil, nil
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	bound, err := ownerContext(ctx, s.previewAuthority, workspace, stored.OwnerID.UUID)
	if err != nil {
		return nil, err
	}
	reader, ok := principal.Actor(bound)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	return []principal.Principal{reader}, nil
}
