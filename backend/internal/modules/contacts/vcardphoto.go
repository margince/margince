// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AwaitsApproval reports whether the card goes to the approval queue instead of
// being written. The browser upload and the mail worker both stage on this, so
// there is one definition of the queue's entry condition.
func (o VCardOutcome) AwaitsApproval() bool {
	return o == VCardNeedsReview || o == VCardProposed
}

// CardPhotoMaxCards bounds the cards one photo may yield. A photo is attended
// work, so the cap is far under vcardMaxCards. It still bounds the writes. A
// reading above it is refused whole and never cut.
const CardPhotoMaxCards = 20

// ProposeCardsFromPhoto runs cards a model read off a photo through the import.
// It refuses an entry not marked ModelRead. Without that check this entry point
// could carry a typed card past the approval a model-read card needs.
func (s *Store) ProposeCardsFromPhoto(ctx context.Context, entries []VCardEntry) ([]VCardResult, error) {
	if len(entries) > CardPhotoMaxCards {
		return nil, &TooManyCardsError{Limit: CardPhotoMaxCards}
	}
	for i := range entries {
		if !entries[i].ModelRead {
			return nil, errors.New("contacts: a card from a photo must be marked as read by a model")
		}
	}
	return s.importCardsFrom(ctx, entries, ids.Nil)
}
