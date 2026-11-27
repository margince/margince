// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

func TestAnInventedOrMismatchedSalutationIsReplacedWithTheStoredGreeting(t *testing.T) {
	for _, tc := range []struct{ language, choice, opening, want string }{
		{"de", "ms", "Sehr geehrter Herr Müller,", "Sehr geehrte Frau Müller,"},
		{"de", "none", "Sehr geehrte Frau Müller,", "Guten Tag Dana Müller,"},
		{"en", "", "Dear Mr Müller,", "Dear Dana Müller,"},
		{"en", "mr", "Dear Ms Müller,", "Dear Mr Müller,"},
		{"en", "ms", "Dear Ms Müller,", "Dear Ms Müller,"},
	} {
		t.Run(tc.language+tc.choice+tc.opening, func(t *testing.T) {
			envelope := draftfloor.Envelope{Language: tc.language, GreetingRegister: "formal", Salutation: tc.choice}
			got := draftcheck.EnsureGreeting(tc.opening+"\n\nThe documents are attached.", envelope, draftcheck.Grounds{FirstName: "Dana", LastName: "Müller"})
			want := tc.want + "\n\nThe documents are attached."
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
