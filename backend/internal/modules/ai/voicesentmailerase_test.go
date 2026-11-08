// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

func TestAFullNameIsMatchedAsWholeWordsAndAOneWordNameNotAtAll(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, want string
		ok         bool
	}{
		{"Anna Berg", `(?<![[:alnum:]_])Anna\s+Berg(?![[:alnum:]_])`, true},
		{"  Jean-Luc   O.Neil ", `(?<![[:alnum:]_])Jean-Luc\s+O\.Neil(?![[:alnum:]_])`, true},
		{"Mary J Blige", `(?<![[:alnum:]_])Mary\s+J\s+Blige(?![[:alnum:]_])`, true},
		{"Li X", `(?<![[:alnum:]_])Li\s+X(?![[:alnum:]_])`, true},
		{"Anna", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := wholeNamePattern(c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("wholeNamePattern(%q) = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestAnAddressIsBoundedOnlyByWhatWouldMakeItAnotherAddress(t *testing.T) {
	t.Parallel()
	want := `(?<![[:alnum:].+-])ann@example\.com(?=$|[^[:alnum:].-]|\.(?:$|[^[:alnum:]]))`
	if got := wholeAddressPattern("ann@example.com"); got != want {
		t.Errorf("wholeAddressPattern = %q, want %q", got, want)
	}
}
