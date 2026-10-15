// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// "Last used" is read off the passport row. The authentication path both doors
// share stamps it, and a connection's row shows the newest stamp across every
// credential it has been through.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *revocationEnv) lastUsed(t *testing.T, passport ids.PassportID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT last_used_at FROM passport WHERE id = $1`, passport).Scan(&at); err != nil {
		t.Fatalf("reading last_used_at: %v", err)
	}
	return at
}

func TestAuthenticatingStampsThePassportOnceAMinute(t *testing.T) {
	e := setupRevocationEnv(t, "passport-last-used-debounce")
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("minting a passport: %v", err)
	}
	if e.lastUsed(t, issued.ID) != nil {
		t.Fatal("a passport nobody has used already carries a last-used time")
	}

	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token); err != nil {
		t.Fatalf("authenticating: %v", err)
	}
	first := e.lastUsed(t, issued.ID)
	if first == nil {
		t.Fatal("authenticating left no last-used time")
	}

	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token); err != nil {
		t.Fatalf("authenticating again: %v", err)
	}
	if again := e.lastUsed(t, issued.ID); !again.Equal(*first) {
		t.Errorf("a second call inside the minute moved the stamp from %v to %v", first, again)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE passport SET last_used_at = now() - interval '5 minutes' WHERE id = $1`, issued.ID); err != nil {
		t.Fatalf("ageing the stamp: %v", err)
	}
	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), issued.Token); err != nil {
		t.Fatalf("authenticating after the minute: %v", err)
	}
	if later := e.lastUsed(t, issued.ID); later.Before(first.Add(-time.Second)) || time.Since(*later) > time.Minute {
		t.Errorf("a call after the minute left the stamp at %v", later)
	}
}

func TestTheSchedulerResolvingAPassportDoesNotCountAsUse(t *testing.T) {
	e := setupRevocationEnv(t, "passport-last-used-scheduler")
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.member, IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("minting a passport: %v", err)
	}

	if _, err := e.svc.AuthenticateAgentByID(e.wsCtx(e.member), issued.ID); err != nil {
		t.Fatalf("resolving the passport by id: %v", err)
	}

	if e.lastUsed(t, issued.ID) != nil {
		t.Error("a background run holding no token stamped the passport as used by an agent")
	}
}

func TestAConnectionShowsTheNewestUseAcrossItsRotatedPassports(t *testing.T) {
	e := setupRevocationEnv(t, "passport-last-used-connection")
	fixture := e.connectOAuthFor(t, e.member)
	old := e.rotateIssuing(t, &fixture)
	if _, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), old.Token); err != nil {
		t.Fatalf("authenticating the first credential: %v", err)
	}
	current := e.rotateIssuing(t, &fixture)

	rows, err := e.svc.ListPassports(e.wsCtx(e.member), e.member)
	if err != nil {
		t.Fatalf("listing passports: %v", err)
	}
	var found bool
	for _, row := range rows {
		if row.ID == current.ID {
			found = true
			if row.LastUsedAt == nil {
				t.Error("the connection reads as never used although its previous credential was")
			}
		}
	}
	if !found {
		t.Fatal("the connection's newest passport is not listed")
	}
}
