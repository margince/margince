// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"testing"
)

func TestANeverExpiringPassportAuthenticatesAndListsWithNoExpiry(t *testing.T) {
	e := setupRevocationEnv(t, "passport-never-expires")
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Scopes: []string{"read"}, NeverExpires: true})
	if err != nil {
		t.Fatalf("minting a never-expiring passport: %v", err)
	}
	if issued.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want none", *issued.ExpiresAt)
	}
	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token); err != nil {
		t.Fatalf("a never-expiring passport did not authenticate: %v", err)
	}
	rows, err := e.svc.ListPassports(e.wsCtx(e.member), e.member)
	if err != nil {
		t.Fatalf("listing passports: %v", err)
	}
	for _, row := range rows {
		if row.ID == issued.ID && row.ExpiresAt != nil {
			t.Fatalf("the list shows expiry %v for a never-expiring passport", *row.ExpiresAt)
		}
	}
}

func TestARevokedNeverExpiringPassportIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "passport-never-expires-revoked")
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Scopes: []string{"read"}, NeverExpires: true})
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := e.svc.RevokePassport(e.wsCtx(e.member), e.member, issued.ID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token); err == nil {
		t.Fatal("a revoked never-expiring passport still authenticated")
	}
}
