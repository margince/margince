// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// maxHandledOffset bounds how far into the window a cursor may point. Each page
// re-reads the window up to its offset, so the bound is also the read's cost.
const maxHandledOffset = 5000

// handledCursor is a page position in Done for you: how many receipts came
// before, the window, and the newest receipt the first page read.
//
// An offset, because the list merges two sources and neither holds the other's
// order. The pin keeps it stable. A receipt landing after the first page is left
// out of every later one, so it cannot push an unread receipt onto a read page.
// The pin is the database's own timestamp, since this clock runs behind it.
type handledCursor struct {
	Offset int       `json:"n"`
	Since  time.Time `json:"s"`
	Newest time.Time `json:"a"`
}

func decodeHandledCursor(token *string) (handledCursor, error) {
	if token == nil || *token == "" {
		return handledCursor{}, nil
	}
	position, err := storekit.DecodeOpaque[handledCursor](*token)
	if err != nil {
		return handledCursor{}, err
	}
	if position.Offset <= 0 || position.Offset > maxHandledOffset || position.Since.IsZero() || position.Newest.IsZero() {
		return handledCursor{}, fmt.Errorf("%w: cursor", apperrors.ErrInvalidArgument)
	}
	return position, nil
}

// pin drops what arrived after the first page was read. The first page itself
// is unpinned: it is what sets the pin.
func (c handledCursor) pin(recent []Receipt) []Receipt {
	if c.Newest.IsZero() {
		return recent
	}
	kept := recent[:0:0]
	for _, receipt := range recent {
		if !receipt.OccurredAt.After(c.Newest) {
			kept = append(kept, receipt)
		}
	}
	return kept
}

// after is the cursor for the page that follows one of `drawn` receipts.
func (c handledCursor) after(drawn int, recent []Receipt) (string, error) {
	next := handledCursor{Offset: c.Offset + drawn, Since: c.Since, Newest: c.Newest}
	if next.Newest.IsZero() && len(recent) > 0 {
		next.Newest = recent[0].OccurredAt
	}
	return storekit.EncodeOpaque(next)
}
