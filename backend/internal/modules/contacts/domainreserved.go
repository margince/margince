// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// PendingReservedDomain keeps reserved names recoverable without automatic enrichment.
const PendingReservedDomain = "reserved_domain"

// WithholdReservedDomain pauses automatic identification without discarding the question.
func (s *Store) WithholdReservedDomain(ctx context.Context, domain string) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return err
	}
	if !values.IsReservedHost(domain) {
		return apperrors.ErrConflict
	}
	return s.withholdDomain(ctx, domain, PendingReservedDomain, "Reserved domain: automatic company identification is paused.", ids.Nil)
}
