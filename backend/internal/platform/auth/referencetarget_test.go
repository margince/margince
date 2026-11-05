// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAMissingReferenceNamesTheFieldAndStaysNotFoundForOtherCallers(t *testing.T) {
	cause := fmt.Errorf("stage is gone: %w", apperrors.ErrNotFound)
	err := NameMissingReference("stage_id", cause)

	fault, ok := errors.AsType[*ReferenceNotFoundError](err)
	if !ok {
		t.Fatalf("NameMissingReference(not found) = %v, want a ReferenceNotFoundError", err)
	}
	field, code, _ := fault.FieldFault()
	if field != "stage_id" || code != "reference_not_found" {
		t.Fatalf("fault = %s/%s, want stage_id/reference_not_found", field, code)
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("an import row or an agent tool must still read this as not-found")
	}
	if !errors.Is(err, cause) {
		t.Fatal("the typed cause must stay reachable through the wrapper")
	}
}

func TestAReferenceProbeAnswerOtherThanNotFoundPassesThrough(t *testing.T) {
	for name, answer := range map[string]error{
		"nil":               nil,
		"permission denied": apperrors.ErrPermissionDenied,
		"infrastructure":    errors.New("connection reset"),
	} {
		t.Run(name, func(t *testing.T) {
			if got := NameMissingReference("company_id", answer); got != answer {
				t.Fatalf("NameMissingReference(%v) = %v, want it untouched", answer, got)
			}
		})
	}
}
