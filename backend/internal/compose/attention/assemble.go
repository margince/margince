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
	day, _, err := s.assembleDay(ctx)
	return day, err
}
