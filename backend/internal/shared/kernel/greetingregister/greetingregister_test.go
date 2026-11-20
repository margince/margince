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

func TestSharedTakesFormalWhenTheStoredRegistersDiffer(t *testing.T) {
	t.Parallel()
	f, fam := greetingregister.Formal, greetingregister.Familiar
	cases := map[string]struct {
		each []greetingregister.Register
		want greetingregister.Register
	}{
		"familiar primary with a formal co-recipient": {[]greetingregister.Register{fam, f}, f},
		"formal primary with a familiar co-recipient": {[]greetingregister.Register{f, fam}, f},
		"all familiar":       {[]greetingregister.Register{fam, fam}, fam},
		"unset says nothing": {[]greetingregister.Register{"", fam}, fam},
		"nothing set":        {[]greetingregister.Register{"", ""}, ""},
	}
	for name, tc := range cases {
		if got := greetingregister.Shared(tc.each...); got != tc.want {
			t.Errorf("%s: Shared = %q, want %q", name, got, tc.want)
		}
	}
}
