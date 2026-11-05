// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func skipReservedDomainTriage(ctx context.Context, store *contacts.Store, domain string, skipReserved bool) (bool, error) {
	if !skipReserved || !values.IsReservedHost(domain) {
		return false, nil
	}
	return true, store.WithholdReservedDomain(ctx, domain)
}
