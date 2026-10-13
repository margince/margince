// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"errors"
	"strings"
	"testing"
)

// The contract declares scopes as unique items and a label of at most 120
// characters; a locally minted passport must refuse what the schema forbids.
func TestAPassportIsRefusedAnAmbiguousScopeListOrAnOverlongLabel(t *testing.T) {
	e := setupRevocationEnv(t, "passport-admission")
	human := e.admin

	long := strings.Repeat("é", 121)
	atLimit := strings.Repeat("é", 120)
	cases := []struct {
		name    string
		in      IssuePassportInput
		field   string
		allowed bool
	}{
		{"duplicate scopes", IssuePassportInput{Scopes: []string{"read", "read"}}, "scopes", false},
		{"121-character label", IssuePassportInput{Label: &long, Scopes: []string{"read"}}, "label", false},
		{"120-character label", IssuePassportInput{Label: &atLimit, Scopes: []string{"read"}}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.IssuePassport(e.wsCtx(human), human, tc.in)
			if tc.allowed {
				if err != nil {
					t.Fatalf("a valid request was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("the request was accepted, want a refusal naming %q", tc.field)
			}
			var refused *InvalidPassportFieldError
			if !errors.As(err, &refused) || refused.Field != tc.field {
				t.Fatalf("refusal %v does not name %q", err, tc.field)
			}
		})
	}
}
