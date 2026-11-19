// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package greetingregister_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAddressMapsEachRegisterToItsGermanForm(t *testing.T) {
	t.Parallel()
	cases := map[greetingregister.Register]textlang.Register{
		greetingregister.Formal:   textlang.RegisterSie,
		greetingregister.Familiar: textlang.RegisterDu,
		"":                        textlang.RegisterUnknown,
		"casual":                  textlang.RegisterUnknown,
	}
	for in, want := range cases {
		if got := greetingregister.Address(in); got != want {
			t.Errorf("Address(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidAcceptsOnlyTheColumnsValues(t *testing.T) {
	t.Parallel()
	for _, ok := range []greetingregister.Register{"formal", "familiar"} {
		if !greetingregister.Valid(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []greetingregister.Register{"", "Sie", "casual"} {
		if greetingregister.Valid(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
