// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (w attentionWaiting) waitingSuppliers(ctx context.Context, rows []activities.WaitingReply) (map[ids.UUID]bool, error) {
	if w.contacts == nil {
		return map[ids.UUID]bool{}, nil
	}
	contactIDs, companyIDs := map[ids.UUID]struct{}{}, map[ids.UUID]struct{}{}
	for _, row := range rows {
		if row.ContactID != (ids.UUID{}) {
			contactIDs[row.ContactID] = struct{}{}
		}
		if row.CompanyID != (ids.UUID{}) {
			companyIDs[row.CompanyID] = struct{}{}
		}
	}
	employers, err := w.contacts.CurrentEmployers(ctx, slices.Collect(maps.Keys(contactIDs)))
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, err
	}
	for _, employer := range employers {
		companyIDs[ids.UUID(employer.CompanyId)] = struct{}{}
	}
	suppliers, err := w.contacts.SupplierCompanies(ctx, slices.Collect(maps.Keys(companyIDs)))
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return map[ids.UUID]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	for contact, employer := range employers {
		if suppliers[ids.UUID(employer.CompanyId)] {
			suppliers[contact] = true
		}
	}
	return suppliers, nil
}
