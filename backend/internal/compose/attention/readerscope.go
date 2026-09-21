// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHICH reader answers a page, once its scope and owner are resolved.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readerFor answers the narrowed service this page reads through.
//
// The narrowing happens in the stores' own queries rather than by dropping rows
// afterwards, so a page full of colleagues' work is never assembled and then
// cut — and the cut that did happen is the one the reader asked for.
func (s *Service) readerFor(
	ctx context.Context, resolved string, namedOwner ids.UUID,
) (*Service, error) {
	// Deeper than the lane feed reads: a batch row counts a pile, and a count
	// taken from a page of ten would report ten over a hundred and fifty.
	reader := s.countingDecisions()
	switch {
	// A named owner outranks the scope word: "their queue" is a narrower
	// question than any of mine/team/all, and answering the wider one would
	// hand back a page that looks like the rep's day and is not.
	case !namedOwner.IsZero():
		return reader.forOwner(namedOwner), nil
	case mineOnly(resolved):
		return reader.forReader(), nil
	case resolved == scopeUnassigned:
		return reader.forUnowned(), nil
	case resolved == scopeTeam:
		return reader.forNoticeTeam(ctx)
	}
	return reader, nil
}
