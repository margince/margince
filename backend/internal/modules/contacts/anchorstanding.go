// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether this installation has finished describing itself — the one fact about
// the anchor company that is not administered.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// AnchorProfileStanding answers whether the installation has an anchor company
// and whether it carries the minimum a reader may rely on.
//
// NO GRANT IS ASKED, and that is a ruling rather than an omission. The
// administered surface beside it (GetAnchorCompany) reads the profile — the
// legal name, the website, every field an operator typed — and an administrator
// is who that belongs to. This reads neither: it answers a yes/no about
// OURSELVES that several ordinary seats already act on.
//
// The growth fit is the case that decides it. `GrowthFitService.Get` is a
// reading aid any human seat opens on any account, and it caps the band it
// reports when this installation has not described what it offers — so a seat
// that could not resolve this would either be refused a page they may read, or
// silently handed a stronger band than the evidence supports. The onboarding
// conversation asks the same question for the same reason: whether to speak
// about a company that exists or one still being described.
//
// It carries no profile FIELD for exactly that reason. Widening it to return
// the company would make it the administered read under another name, which is
// the borrowing this whole gate change exists to undo.
func (s *Store) AnchorProfileStanding(ctx context.Context) (exists, minimumComplete bool, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		companyID, err := anchorCompany(ctx, tx, false)
		if errors.Is(err, apperrors.ErrNotFound) {
			// An installation that has not described itself yet. Not an error:
			// every caller here treats it as "not confirmed", which is the
			// honest reading of a profile nobody has written.
			return nil
		}
		if err != nil {
			return err
		}
		company, err := readAnchorCompany(ctx, tx, companyID)
		if err != nil {
			return err
		}
		exists, minimumComplete = true, company.MinimumComplete
		return nil
	})
	if err != nil {
		return false, false, err
	}
	return exists, minimumComplete, nil
}
