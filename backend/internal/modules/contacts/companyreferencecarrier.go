// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyReferenceCarrier lets a module move its company references within the merge transaction.
type CompanyReferenceCarrier func(context.Context, pgx.Tx, ids.UUID, ids.UUID) error

// WithCompanyReferenceCarrier lets the owning module carry company references during merges.
func (s *Store) WithCompanyReferenceCarrier(carry CompanyReferenceCarrier) *Store {
	s.companyReferenceCarrier = carry
	return s
}

// WithCompanyReferenceCarrier lets the owning module carry company references during merges.
func (p *Provider) WithCompanyReferenceCarrier(carry CompanyReferenceCarrier) *Provider {
	p.store.WithCompanyReferenceCarrier(carry)
	return p
}

// WithCompanyReferenceCarrier lets the owning module carry company references during merges.
func (h Handlers) WithCompanyReferenceCarrier(carry CompanyReferenceCarrier) Handlers {
	h.store.WithCompanyReferenceCarrier(carry)
	return h
}
