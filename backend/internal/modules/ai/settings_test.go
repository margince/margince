// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// A chat provider added with a BYOK key needs a declared setting, or the key
// is neither documented nor marked secret.
func TestEveryChatProviderKeyIsADeclaredSecret(t *testing.T) {
	t.Parallel()
	var want, got []string
	for _, provider := range knownProviders {
		if env, cloud := cloudKeyEnv[provider]; cloud {
			want = append(want, env)
		}
	}
	for _, s := range Settings() {
		if !s.Secret() {
			t.Errorf("%s is a credential and is not secret", s.Env())
		}
		got = append(got, s.Env())
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("declared keys %v, chat providers read %v", got, want)
	}
}
