// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"errors"
	"testing"
)

// What no withdrawal link can be written for, refused before either door spends
// a probe on it.
//
// Both doors share this validator, so a refusal that quietly stopped refusing
// would reach the insert on both: a credential with no address is a link with
// nowhere to send it, and a credential with no recognised scope is one the
// press cannot bound — which is how an all-marketing link comes to stop
// business correspondence.
func TestAWithdrawalMintRefusesWhatNoLinkCanBeWrittenFor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    WithdrawalMintInput
		field string
	}{
		{"no address at all", WithdrawalMintInput{Scope: WithdrawalScopeAllMarketing}, "address"},
		{"an address of blanks", WithdrawalMintInput{Address: "   ", Scope: WithdrawalScopeAllMarketing}, "address"},
		{"no scope", WithdrawalMintInput{Address: "someone@example.test"}, "scope"},
		{"a scope neither press knows", WithdrawalMintInput{Address: "someone@example.test", Scope: "everything"}, "scope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var refusal *ValidationError
			if err := validWithdrawalMint(tc.in); !errors.As(err, &refusal) {
				t.Fatalf("refusal = %v, want a ValidationError naming %q", err, tc.field)
			}
			if refusal.Field != tc.field {
				t.Errorf("the refusal names %q, want %q — a caller cannot fix what it is not told about",
					refusal.Field, tc.field)
			}
		})
	}
	// The positive control. Without it every case above would also pass against
	// a validator that refused everything, which is the one way this test could
	// be green over a mint nobody can reach.
	if err := validWithdrawalMint(WithdrawalMintInput{
		Address: "Someone@Example.Test", Scope: WithdrawalScopeAllMarketing,
	}); err != nil {
		t.Errorf("an all-marketing mint for a real address was refused: %v", err)
	}
}
