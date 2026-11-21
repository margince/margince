// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturepurgebind

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestCheckRefusesAnySelectionThatDiffersFromThePreview(t *testing.T) {
	seen := &Counts{Destroyed: 0, Released: 1, Anonymised: 2}
	if err := seen.Check(0, 1, 2); err != nil {
		t.Errorf("the previewed selection was refused: %v", err)
	}
	for _, now := range [][3]int{{1, 1, 2}, {0, 2, 2}, {0, 1, 3}} {
		err := seen.Check(now[0], now[1], now[2])
		if !errors.Is(err, ErrSelectionChanged) || !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("selection %v: err = %v, want a conflict", now, err)
		}
	}
	var unbound *Counts
	if err := unbound.Check(5, 5, 5); err != nil {
		t.Errorf("no expectation must bind nothing: %v", err)
	}
}
