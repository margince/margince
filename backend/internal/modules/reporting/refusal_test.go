// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestRefusedFieldNamesTheFieldAndKeepsTheHelpersSentence(t *testing.T) {
	err := refusedField("scope", invalid("an owner scope names which owner"))
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 {
		t.Fatalf("want one 422 field, got %+v", fault)
	}
	if got := fault.Fields[0]; got.Field != "scope" || got.Message != "an owner scope names which owner" {
		t.Fatalf("field refusal lost its sentence: %+v", got)
	}
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatal("a refusal must still unwrap to the invalid-argument sentinel")
	}
}

func TestRefusedFieldLeavesOtherFailuresAlone(t *testing.T) {
	if got := refusedField("scope", apperrors.ErrNotFound); !errors.Is(got, apperrors.ErrNotFound) {
		t.Fatalf("a missing row became %v", got)
	}
	named := refuse("value", "out_of_range", "enter a non-negative target")
	if got := refusedField("scope", named); got != named {
		t.Fatalf("an already named refusal was renamed: %v", got)
	}
}
