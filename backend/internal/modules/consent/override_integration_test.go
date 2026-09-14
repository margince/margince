// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The writer door for a rep's standing override, against a real database.
//
// Integration rather than unit, for the same reason suppress_integration_test.go
// gives: liveOverride reads the row back and applies it to every future send in
// its category, so a write that landed with the wrong category, the wrong level
// or no row at all is a defect no fake store could show.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// liveOverrideRow reads back what the write actually stored.
func liveOverrideRow(t *testing.T, e *channelConsentEnv, contact ids.ContactID) (category, level, reason string) {
	t.Helper()
	err := e.owner.QueryRow(context.Background(), `
		SELECT category, decided_by_level, reason
		  FROM communication_override
		 WHERE contact_id = $1 AND revoked_at IS NULL`, contact).Scan(&category, &level, &reason)
	if err != nil {
		t.Fatalf("reading back the override: %v", err)
	}
	return category, level, reason
}

// opsCtx builds a context for a seat holding the "ops" role — a role the
// fixture harness has no helper for, and the one this file needs: authorityOf
// answers LevelUser for anything that is not the LITERAL admin role, and "ops"
// is what proves that reading auth.RequireAdmin rather than trusting a role
// name that merely sounds unprivileged, like "rep".
func opsCtx(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"ops"},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestAllowRecordsAUserLevelOverride is the capability that did not exist: a
// rep vouches, and the row lands at their own level, liftable by an admin —
// the same shape TestARepsRowIsWrittenAtTheRepsOwnLevel holds for a stop.
func TestAllowRecordsAUserLevelOverride(t *testing.T) {
	e := setupChannelConsent(t)

	// A contact this rep owns. EnsureWritable's write-authority arm refuses a
	// bounded seat on somebody else's record, so a rep arm that used the
	// shared fixture would be testing the refusal rather than the level.
	own := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Allow(boundedRepCtx(e.ws, e.user), AllowInput{
		ContactID: own, Category: "marketing", Reason: "the lead confirmed by phone",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	category, level, reason := liveOverrideRow(t, e, own)
	if category != "marketing" {
		t.Errorf("category = %q, want marketing", category)
	}
	if level != string(commsauthz.LevelUser) {
		t.Errorf("decided_by_level = %q, want user for a rep seat", level)
	}
	if reason != "the lead confirmed by phone" {
		t.Errorf("reason = %q, want the reason the rep gave", reason)
	}
}

// TestAllowRecordsAnAdminLevelOverride is the other seat: the harness's
// default context binds an admin, so its row must land at admin — not because
// admin outranks user in the abstract, but because that is what the level
// system promises the next reader of this contact's history.
func TestAllowRecordsAnAdminLevelOverride(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "customer_service", Reason: "confirmed with the account holder",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	_, level, _ := liveOverrideRow(t, e, e.contact)
	if level != string(commsauthz.LevelAdmin) {
		t.Errorf("decided_by_level = %q, want admin for an admin seat", level)
	}
}

// TestAllowTakesLevelFromThePrincipalNotTheBody is the point of the whole
// design: AllowInput carries no level field at all, so nothing a caller sends
// can choose one. An "ops" seat — neither "rep" nor "admin" — is what proves
// the level comes from authorityOf's literal admin check rather than a role
// name that happens to sound like a rep.
func TestAllowTakesLevelFromThePrincipalNotTheBody(t *testing.T) {
	e := setupChannelConsent(t)

	if err := e.store.Allow(opsCtx(e.ws, e.user), AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "ops confirmed with the account holder",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	_, level, _ := liveOverrideRow(t, e, e.contact)
	if level != string(commsauthz.LevelUser) {
		t.Errorf("decided_by_level = %q, want user: an ops seat is not the literal admin role", level)
	}
}

// TestAllowRequiresAReason is the difference from Suppress: this write
// overrules the engine's own answer, and the reason is the whole point of a
// human record able to say why.
func TestAllowRequiresAReason(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "marketing"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("an empty reason was refused with %v, want a validation error", err)
	}
	if invalid.Field != fieldReason {
		t.Errorf("refused on field %q, want %q", invalid.Field, fieldReason)
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_override WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a refused reason still wrote %d row(s)", rows)
	}
}

// TestAllowRejectsAnUnknownCategory bounds the door to what the engine can
// actually resolve a send to — a vouch for a category that does not exist is
// a promise the gate could never redeem.
func TestAllowRejectsAnUnknownCategory(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "nope", Reason: "a reason"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("an unknown category was refused with %v, want a validation error", err)
	}
	if invalid.Field != "category" {
		t.Errorf("refused on field %q, want %q", invalid.Field, "category")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_override WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a refused category still wrote %d row(s)", rows)
	}
}

// TestAllowThenTheSendGoesThrough is the end-to-end proof: the writer door
// this file tests and the reader door override_gate_integration_test.go
// already proved are the same table, so a rep's own Allow call must flip the
// same refusal that file seeded directly.
func TestAllowThenTheSendGoesThrough(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	req := commsauthz.Request{LegacyPurposeKey: "newsletter"}

	// BEFORE the override: no consent on file, so the engine cannot support a
	// marketing send on its own reading. Without this the test would prove
	// nothing about the write actually reaching the gate.
	before := e.decide(t, req)
	if before.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow before any override was recorded — the fixture proves nothing")
	}

	// A separate context from e.ctx: the harness's decide() context holds only
	// contact.Read (it never needed to write), and Allow's own door requires
	// contact.Update at auth.Require. e.ctx itself is left untouched so
	// decide() below still runs the same session every other resolve test does.
	writerCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	writerCtx = principal.WithCorrelationID(writerCtx, ids.NewV7())
	writerCtx = principal.WithActor(writerCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := e.store.Allow(writerCtx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "the buyer confirmed by phone",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	after := e.decide(t, req)
	if after.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: a live override should have vouched for this send",
			after.Verdict, after.ReasonCode)
	}
	if after.ReasonCode != commsauthz.ReasonAllowedByOverride {
		t.Errorf("reason = %q, want %q", after.ReasonCode, commsauthz.ReasonAllowedByOverride)
	}
}
