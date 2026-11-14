// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Store) ContactReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	return s.recordReferences(ctx, contactEntity, want)
}

func (s *Store) CompanyReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	return s.recordReferences(ctx, companyEntity, want)
}

func (s *Store) LeadReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	return s.recordReferences(ctx, leadEntity, want)
}

func (s *Store) recordReferences(ctx context.Context, object string, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	rows, err := s.recordIdentities(ctx, object, want, storekit.IncludeArchived)
	if err != nil {
		return nil, err
	}
	references := make([]crmcontracts.RecordReference, 0, len(rows))
	for _, row := range rows {
		reference := row.reference
		if object == leadEntity {
			lead := crmcontracts.Lead{FullName: reference.Name}
			if row.email != nil {
				lead.Email = new(openapi_types.Email(strings.TrimSpace(*row.email)))
			}
			name := leadIdentityName(lead)
			reference.Name = nil
			if name != "" {
				reference.Name = &name
			}
		}
		if object == companyEntity {
			reference.Logo = LogoURL(ids.UUID(reference.Id), row.logoKey, LogoWide)
		}
		references = append(references, reference)
	}
	return references, nil
}
