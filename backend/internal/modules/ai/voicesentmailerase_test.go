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
		{"Anna Berg", `\mAnna\s+Berg\M`, true},
		{"  Jean-Luc   O.Neil ", `\mJean-Luc\s+O\.Neil\M`, true},
		{"Mary J Blige", `\mMary\s+J\s+Blige\M`, true},
		{"Li X", `\mLi\s+X\M`, true},
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
	want := `(?<![[:alnum:]._%+-])ann@example\.com(?=$|[^[:alnum:].-]|\.(?:$|[^[:alnum:]]))`
	if got := wholeAddressPattern("ann@example.com"); got != want {
		t.Errorf("wholeAddressPattern = %q, want %q", got, want)
	}
}
