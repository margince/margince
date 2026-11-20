// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package employerchange

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// Register puts the accept effect and the edit refusal on the deciding
// approvals service. A declined question needs no effect: the inbox remembers
// it per pair.
func Register(svc *approvals.Service, store *contacts.Store) {
	svc.WithEffect(Kind, Effect(svc, store))
	svc.WithPrecheck(Kind, Precheck)
}
