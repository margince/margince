// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGoogleSignInOptionsRefusesAMalformedPublicBaseURL(t *testing.T) {
	cfg := apiConfig{
		gmailClientID:     "cid",
		gmailClientSecret: "secret",
		connectorStateKey: "0123456789012345678901234567890123",
		publicBaseURL:     "https://admin:hunter2@crm.example.com",
	}
	if _, err := googleSignInOptions(cfg, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a refusal for a base URL carrying userinfo")
	}
}

func TestGoogleSignInOptionsAcceptsABareOrigin(t *testing.T) {
	cfg := apiConfig{
		gmailClientID:     "cid",
		gmailClientSecret: "secret",
		connectorStateKey: "0123456789012345678901234567890123",
		publicBaseURL:     "https://crm.example.com",
	}
	var stdout bytes.Buffer
	opts, err := googleSignInOptions(cfg, &stdout)
	if err != nil {
		t.Fatalf("googleSignInOptions: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("opts = %v, want exactly one Option", opts)
	}
	if !strings.Contains(stdout.String(), "google sign-in enabled") {
		t.Fatalf("boot log = %q, want the enabled line", stdout.String())
	}
}

func TestGoogleSignInOptionsSkipsValidationWhenUnconfigured(t *testing.T) {
	// No client id at all — the malformed base URL below must never be
	// inspected, matching gmailOptions' own posture of validating only what
	// it is actually about to wire.
	cfg := apiConfig{publicBaseURL: "https://admin:hunter2@crm.example.com"}
	if _, err := googleSignInOptions(cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("googleSignInOptions on an unconfigured deployment: %v", err)
	}
}
