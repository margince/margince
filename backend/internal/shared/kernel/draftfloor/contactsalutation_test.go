// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

func TestFormalGreetingsUseOnlyAnExplicitSalutation(t *testing.T) {
	for _, tc := range []struct{ lang, register, salutation, want string }{
		{"de", "formal", "mr", "Sehr geehrter Herr Müller,"},
		{"de", "formal", "ms", "Sehr geehrte Frau Müller,"},
		{"en", "formal", "mr", "Dear Mr Müller,"},
		{"en", "formal", "ms", "Dear Ms Müller,"},
		{"de", "formal", "none", "Guten Tag Dana Müller,"},
		{"de", "formal", "", "Guten Tag Dana Müller,"},
		{"de", "familiar", "ms", "Hallo Dana,"},
		{"en", "familiar", "mr", "Hi Dana,"},
	} {
		t.Run(tc.lang+tc.register+tc.salutation, func(t *testing.T) {
			envelope := draftfloor.Envelope{Language: tc.lang, GreetingRegister: tc.register, Salutation: tc.salutation}
			if got := envelope.Greeting("Dana", "Müller"); got != tc.want {
				t.Fatalf("greeting %q, want %q", got, tc.want)
			}
		})
	}
}
