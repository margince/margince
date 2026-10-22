// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Every field of the contract's company draft survives the trip into the
// stored draft and back. A field the contract gains cannot be accepted and
// dropped, since the census is the contract type itself.
func TestEveryOnboardingDraftFieldIsKept(t *testing.T) {
	t.Parallel()
	var sent crmcontracts.OnboardingCompanyDraft
	value := reflect.ValueOf(&sent).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() != reflect.Pointer || field.Type().Elem().Kind() != reflect.String {
			t.Fatalf("draft field %s is %s; this census reads only optional strings, so widen it",
				value.Type().Field(i).Name, field.Type())
		}
		name := value.Type().Field(i).Name
		field.Set(reflect.ValueOf(&name))
	}
	if kept := onboardingDraftResponse(onboardingDraftInput(sent)); !reflect.DeepEqual(kept, sent) {
		t.Errorf("the draft came back as %+v, want every field it was sent: %+v", kept, sent)
	}
}
