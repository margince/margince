// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestSteeringTrimsFieldsAndTreatsEmptyAsAbsent(t *testing.T) {
	t.Parallel()
	padded, blank := "  shorter \n", " \t"
	got := mustSteer(t, &padded, &padded, &padded, nil)
	if got.Intent != "shorter" || got.RewriteOf != "shorter" || got.Subject != "shorter" {
		t.Errorf("trimmed steering = %+v, want each field %q", got, "shorter")
	}
	if got := mustSteer(t, &blank, &blank, nil, nil); got != (draftfloor.Steering{}) {
		t.Errorf("blank fields = %+v, want a first draft with no ask", got)
	}
	if got := mustSteer(t, nil, nil, nil, nil); got != (draftfloor.Steering{}) {
		t.Errorf("absent fields = %+v, want zero", got)
	}
}

func TestSteeringCarriesAShippedLanguageAndRefusesAnyOther(t *testing.T) {
	t.Parallel()
	for _, lang := range textlang.Shipped {
		code := string(lang)
		if got := mustSteer(t, nil, nil, nil, &code); got.Language != lang {
			t.Errorf("chosen %q = %q, want it carried as chosen", code, got.Language)
		}
	}
	for _, code := range []string{"fr", "", "DE", " de"} {
		_, err := draftfloor.NewSteering(nil, nil, nil, &code)
		fault, ok := errors.AsType[apperrors.FieldFault](err)
		if !ok {
			t.Errorf("chosen %q = %v, want a field refusal", code, err)
			continue
		}
		if field, _, message := fault.FieldFault(); field != "language" || message == "" {
			t.Errorf("chosen %q refused on %q with %q, want language and what to send", code, field, message)
		}
	}
}

func mustSteer(t *testing.T, intent, rewriteOf, subject, language *string) draftfloor.Steering {
	t.Helper()
	steering, err := draftfloor.NewSteering(intent, rewriteOf, subject, language)
	if err != nil {
		t.Fatalf("steering refused: %v", err)
	}
	return steering
}
