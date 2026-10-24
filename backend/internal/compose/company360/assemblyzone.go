// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// zone is the installation zone every day count on the page is reckoned in,
// read once per assembly like the language.
func (a *assembly) zone() (*time.Location, error) {
	if a.loc == nil {
		loc, err := identity.ZoneAppliedTx(a.ctx, a.tx)
		if err != nil {
			return nil, err
		}
		a.loc = loc
	}
	return a.loc, nil
}
