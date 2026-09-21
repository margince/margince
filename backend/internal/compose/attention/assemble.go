// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The page's entry point, and the one admission its lanes do not each own.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// Assemble reads every lane and returns the day.
//
// A lane whose read is REFUSED is omitted and named rather than reported empty.
// Any other failure is returned: a lane that is broken rather than withheld
// must not read as a clear day.
func (s *Service) Assemble(ctx context.Context) (crmcontracts.Attention, error) {
	// The page is a contact's own day, and every lane is gated for itself —
	// a lane the caller may not read is omitted and named rather than
	// returned empty. This asks the one question no single lane owns: is
	// there a member here at all. Without it the page's admission is the
	// union of fourteen lanes' admissions, which is not a sentence anyone
	// can read at the entry point.
	if err := auth.RequireMember(ctx); err != nil {
		return crmcontracts.Attention{}, err
	}
	// ONE snapshot for every lane. Read lane-by-lane the page could contradict
	// itself — a deal that closed between lane 3 and lane 11 appeared in one
	// and not the other — and it paid a transaction per lane to do it.
	var day crmcontracts.Attention
	err := s.inSnapshot(ctx, func(ctx context.Context) error {
		var err error
		day, _, err = s.assembleDay(ctx)
		return err
	})
	return day, err
}
