// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestPreflightInstrumentationAdmitsAUnitWithNone(t *testing.T) {
	if err := preflightInstrumentation(extension.Extension{Name: "alpha"}); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightInstrumentationRefusesAnEmptyDeclaration(t *testing.T) {
	err := preflightInstrumentation(extension.Extension{Name: "alpha", Instrumentation: &extension.Instrumentation{}})
	if !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
