// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// A provider added with a BYOK key needs a declared setting, or the key is
// neither documented nor marked secret. Decision providers take keys too.
func TestEveryProviderKeyIsADeclaredSecret(t *testing.T) {
	t.Parallel()
	var want, got []string
	for _, provider := range providerNames() {
		if env, cloud := cloudKeyEnv[provider]; cloud {
			want = append(want, env)
		}
	}
	for _, s := range Settings() {
		if !s.Secret() {
			t.Errorf("%s is a credential and is not secret", s.Env())
		}
		if !s.Stored() {
			t.Errorf("%s is a credential an admin cannot set", s.Env())
		}
		got = append(got, s.Env())
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("declared keys %v, providers read %v", got, want)
	}
}
