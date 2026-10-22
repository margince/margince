// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestAFullNameMustBeTheFirstAndLastNameInEitherOrder(t *testing.T) {
	t.Parallel()
	name := func(s string) *string { return &s }
	for _, c := range []struct {
		label       string
		full        string
		first, last *string
		refused     bool
	}{
		{"the two parts joined", "Anna Weber", name("Anna"), name("Weber"), false},
		{"family name first", "Weber Anna", name("Anna"), name("Weber"), false},
		{"case and spacing aside", "  anna   WEBER ", name("Anna"), name("Weber"), false},
		{"the reported row", "Signing-Noreply", name("Contact"), name("A"), true},
		{"only a first name", "Signing-Noreply", name("Contact"), nil, false},
		{"an empty last name", "Signing-Noreply", name("Contact"), name(" "), false},
		{"no full name", "", name("Anna"), name("Weber"), false},
	} {
		err := refuseDisagreeingNames(c.full, c.first, c.last)
		if !c.refused {
			if err != nil {
				t.Errorf("%s: refused with %v, want accepted", c.label, err)
			}
			continue
		}
		var fault *values.ParseError
		if !errors.As(err, &fault) || fault.Field != "full_name" || fault.Code != fullNameDisagreesCode {
			t.Fatalf("%s: err = %v, want a full_name fault %q", c.label, err, fullNameDisagreesCode)
		}
		if !strings.Contains(fault.Message, "Signing-Noreply") || !strings.Contains(fault.Message, "Contact A") {
			t.Errorf("%s: message %q does not name both values", c.label, fault.Message)
		}
	}
}
