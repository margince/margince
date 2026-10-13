// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who is quoted in a voice sample? The ai module strips a quoted speaker's turns
// out of the owner's prose before it enters the corpus, and the names it can
// recognise live in two modules it may not import: the contacts the owner sells
// to and the colleagues they work beside.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// voiceKnownSpeakers answers with every label either module recognises, less
// the caller's own names: a contact or colleague who shares the owner's name
// does not make the owner's own lines a quotation. A caller without the grant
// to read contacts recognises no contact, which is not a failure of the ingest:
// the names it cannot see are names it cannot quote.
func voiceKnownSpeakers(pool *pgxpool.Pool) ai.KnownSpeakers {
	store := contacts.NewStore(InstallationDB(pool))
	seats := identity.NewService(pool)
	return func(ctx context.Context, labels []string) ([]string, error) {
		fromContacts, err := store.ContactNamesAmong(ctx, labels)
		if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, err
		}
		fromColleagues, own, err := seats.SeatNamesAmong(ctx, labels)
		if err != nil {
			return nil, err
		}
		known := make([]string, 0, len(fromContacts)+len(fromColleagues))
		for _, label := range append(fromContacts, fromColleagues...) {
			if !slices.Contains(own, label) {
				known = append(known, label)
			}
		}
		return known, nil
	}
}
