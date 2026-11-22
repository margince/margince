// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestEveryClassifiedTargetTypeIsAKnownTargetType(t *testing.T) {
	t.Parallel()
	for _, classified := range ClassifiedTargetTypes() {
		if err := requireKnownTargetType("target type", classified); err != nil {
			t.Errorf("%s has a visibility rule but cannot be staged: %v", classified, err)
		}
	}
}

func TestAStagingRefusesATypeTheCheckWouldRefuse(t *testing.T) {
	t.Parallel()
	for _, accepted := range []string{"", "deal", "stage", "ext_visit_log_visit"} {
		if err := requireKnownTargetType("target type", accepted); err != nil {
			t.Errorf("%q was refused: %v", accepted, err)
		}
	}
	for _, refused := range []string{"widget", "Deal", "ext:visit-log:visit", "ext_Bad"} {
		if err := requireKnownTargetType("target type", refused); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%q: want ErrInvalidArgument, got %v", refused, err)
		}
	}
}
