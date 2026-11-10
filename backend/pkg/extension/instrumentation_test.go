// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func validInstrumentation() extension.Instrumentation {
	return extension.Instrumentation{
		Start: func(context.Context, extension.Process) (extension.Instruments, error) {
			return extension.Instruments{}, nil
		},
		Browser: "instrument",
		Edge:    http.NotFoundHandler(),
	}
}

func TestInstrumentationValidateAdmitsAWholeDeclaration(t *testing.T) {
	if err := validInstrumentation().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestInstrumentationValidateAdmitsABrowserOnlyDeclaration(t *testing.T) {
	i := extension.Instrumentation{Browser: "instrument"}
	if err := i.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestInstrumentationValidateRefusals(t *testing.T) {
	cases := map[string]func(*extension.Instrumentation){
		"nothing declared":          func(i *extension.Instrumentation) { *i = extension.Instrumentation{} },
		"browser export not ident":  func(i *extension.Instrumentation) { i.Browser = "./instrument.ts" },
		"browser export too long":   func(i *extension.Instrumentation) { i.Browser = "a" + strings.Repeat("b", 64) },
		"browser export is default": func(i *extension.Instrumentation) { i.Browser = "default" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			i := validInstrumentation()
			mutate(&i)
			if err := i.Validate(); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
