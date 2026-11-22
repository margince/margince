// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturepurgebind ties a capture purge to the counts a preview showed.
package capturepurgebind

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Counts are the figures a caller confirmed from a preview.
type Counts struct {
	Destroyed, Released, Anonymised int
}

type countsKey struct{}

// ErrSelectionChanged refuses a purge whose selection no longer matches the
// preview, so nothing is destroyed that the caller did not see counted.
var ErrSelectionChanged = fmt.Errorf("%w: the mail this rule matches changed since the preview", apperrors.ErrConflict)

// Bind returns a context carrying the three confirmed counts. All three or
// none: a partial confirmation would bind less than the caller saw.
func Bind(ctx context.Context, destroyed, released, anonymised *int) (context.Context, error) {
	switch {
	case destroyed == nil && released == nil && anonymised == nil:
		return ctx, nil
	case destroyed == nil || released == nil || anonymised == nil:
		return ctx, fmt.Errorf("%w: send expect_destroyed, expect_released and expect_anonymised together", apperrors.ErrInvalidArgument)
	}
	return context.WithValue(ctx, countsKey{}, Counts{Destroyed: *destroyed, Released: *released, Anonymised: *anonymised}), nil
}

// Check answers ErrSelectionChanged when the context carries confirmed counts
// that differ from the current selection. Without them it binds nothing.
func Check(ctx context.Context, destroyed, released, anonymised int) error {
	confirmed, ok := ctx.Value(countsKey{}).(Counts)
	if !ok || confirmed == (Counts{destroyed, released, anonymised}) {
		return nil
	}
	return ErrSelectionChanged
}
