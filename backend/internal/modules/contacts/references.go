// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContactReferences returns visible contact identities, including archived records.
func (s *Store) ContactReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	return s.recordReferences(ctx, contactEntity, want)
}

// CompanyReferences returns visible company identities with revisioned logos, including archives.
func (s *Store) CompanyReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	return s.recordReferences(ctx, companyEntity, want)
}

// LeadReferences uses the detail name/email fallback for visible leads, including archives.
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
