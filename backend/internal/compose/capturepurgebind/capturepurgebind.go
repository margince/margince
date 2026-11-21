// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturepurgebind ties a capture purge to the counts a preview showed.
package capturepurgebind

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Counts are the figures a caller confirmed from a preview.
type Counts struct {
	Destroyed, Released, Anonymised int
}

// ErrSelectionChanged refuses a purge whose selection no longer matches the
// preview, so nothing is destroyed that the caller did not see counted.
var ErrSelectionChanged = fmt.Errorf("%w: the mail this rule matches changed since the preview", apperrors.ErrConflict)

// Check answers ErrSelectionChanged when the current selection differs from
// what was confirmed. A nil expectation binds nothing.
func (c *Counts) Check(destroyed, released, anonymised int) error {
	if c == nil || *c == (Counts{destroyed, released, anonymised}) {
		return nil
	}
	return ErrSelectionChanged
}

// Confirmed reads the three query counts. All three or none: a partial
// confirmation would bind less than the caller saw.
func Confirmed(destroyed, released, anonymised *int) (*Counts, error) {
	switch {
	case destroyed == nil && released == nil && anonymised == nil:
		return nil, nil
	case destroyed == nil || released == nil || anonymised == nil:
		return nil, fmt.Errorf("%w: send expect_destroyed, expect_released and expect_anonymised together", apperrors.ErrInvalidArgument)
	}
	return &Counts{Destroyed: *destroyed, Released: *released, Anonymised: *anonymised}, nil
}
