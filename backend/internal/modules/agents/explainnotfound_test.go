// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type absenceWithAReason struct{}

func (absenceWithAReason) Error() string { return "nothing has been made for today yet" }
func (absenceWithAReason) Unwrap() error { return apperrors.ErrNotFound }
func (absenceWithAReason) MessageFault() (code, message string) {
	return "not_found", "nothing has been made for today yet"
}

func TestANotFoundThatSaysWhySpeaksForItself(t *testing.T) {
	said := notFoundExplanation(fmt.Errorf("reading: %w", absenceWithAReason{}))
	if said != "nothing has been made for today yet" {
		t.Errorf("explained as %q, want its own reason and no row-scope sentence", said)
	}
	if bare := notFoundExplanation(apperrors.ErrNotFound); !strings.Contains(bare, "row scope") {
		t.Errorf("a bare not-found explained as %q, want the row-scope sentence", bare)
	}
}
