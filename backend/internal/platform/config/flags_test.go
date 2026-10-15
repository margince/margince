// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "testing"

func TestFlagsAreTheRegisteredOnesPlusTheLoaderOwn(t *testing.T) {
	reg := mustRegistry(t,
		Define(Spec[string]{Key: "addr", Env: "X_ADDR", Flag: "addr", Type: String, Doc: "d"}),
		Define(Spec[bool]{Key: "inline_relay", Env: "X_INLINE_RELAY", Flag: "inline-relay", Type: Bool, Doc: "d"}),
	)
	got, err := parseFlags(reg, []string{"--addr=:9", "--config", "a.yaml", "--config-file=b.yaml", "--env-file", "c.env", "--config-trace", "--inline-relay"})
	if err != nil {
		t.Fatal(err)
	}
	if got.raw["addr"] != ":9" || got.config != "a.yaml" || got.configFile != "b.yaml" || got.envFile != "c.env" || !got.trace {
		t.Fatalf("got %+v", got)
	}
	if got.raw["inline-relay"] != "true" {
		t.Fatalf("a bare bool flag means true, got %q", got.raw["inline-relay"])
	}
	if _, err := parseFlags(reg, []string{"--adr=:9"}); err == nil {
		t.Fatal("an unknown flag must stop the boot")
	}
}

func mustRegistry(t *testing.T, s ...Any) *Registry {
	t.Helper()
	r, err := NewRegistry(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
