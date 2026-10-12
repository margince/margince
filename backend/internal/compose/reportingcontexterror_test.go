// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAnUnsupportedCloseWindowNamesWhatIsAccepted(t *testing.T) {
	err := errUnsupportedCloseWindow()
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("not an invalid-argument refusal: %v", err)
	}
	for _, accepted := range []string{"all_open", "fiscal_quarter"} {
		if !strings.Contains(err.Error(), accepted) {
			t.Errorf("the refusal does not name %q, so an assistant must guess it: %v", accepted, err)
		}
	}
}
