// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// An undo records a conflict as a row it left alone and carries on; any other
// refusal would stop it.
func TestAnUndoRefusalIsAConflict(t *testing.T) {
	for name, err := range map[string]error{
		"touched by a human": &HumanTouchedError{EntityType: "company"},
		"still referenced":   &StillReferencedError{Deals: 1},
	} {
		if !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("%s: %v is not a conflict", name, err)
		}
	}
}
