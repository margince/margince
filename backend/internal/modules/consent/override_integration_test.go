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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

	if _, err := e.store.Allow(boundedRepCtx(e.ws, e.user), AllowInput{
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

	if _, err := e.store.Allow(e.ctx, AllowInput{
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

	if _, err := e.store.Allow(opsCtx(e.ws, e.user), AllowInput{
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

	_, err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "marketing"})
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

	_, err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "nope", Reason: "a reason"})
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

// TestAllowRejectsASubjectServingCategory bounds the door below Valid: a
// security_notice (or any ServesTheSubject category) is never refused for lack
// of evidence, so a machine refusal never resolves to one and an override
// naming it is a row liveOverride could never match. The door refuses it up
// front rather than writing a dead row.
func TestAllowRejectsASubjectServingCategory(t *testing.T) {
	e := setupChannelConsent(t)

	_, err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "security_notice", Reason: "a reason"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("a subject-serving category was refused with %v, want a validation error", err)
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
		t.Errorf("a refused subject-serving category still wrote %d row(s)", rows)
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
	if _, err := e.store.Allow(writerCtx, AllowInput{
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

// writerCtxAt builds a context whose seat writes at the named role — "rep" for
// LevelUser, "admin" for LevelAdmin — with the contact:Update grant every
// writer door in this file needs and RowScopeAll so ownership never confounds
// the level comparison under test.
func writerCtxAt(ws, user ids.UUID, role string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{role},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestAdminRevokesAUserOverride is the primary case: an admin revokes a rep's
// override, and a subsequent send in that category refuses again — the
// engine re-reads the LIVE rows inside the sending transaction, so the row
// must actually be gone from its point of view, not merely reported as such
// by the revoke call.
func TestAdminRevokesAUserOverride(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	req := commsauthz.Request{LegacyPurposeKey: "newsletter"}

	repCtx := writerCtxAt(e.ws, e.user, "rep")
	if _, err := e.store.Allow(repCtx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "the buyer confirmed by phone",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	afterAllow := e.decide(t, req)
	if afterAllow.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s) after Allow, want allow — the fixture proves nothing otherwise",
			afterAllow.Verdict, afterAllow.ReasonCode)
	}
	overrideID := afterAllow.OverrideID

	adminCtx := writerCtxAt(e.ws, ids.NewV7(), "admin")
	if err := e.store.RevokeOverride(adminCtx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: overrideID, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("an admin revoking a rep's override: %v", err)
	}

	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at IS NULL FROM communication_override WHERE id = $1`, overrideID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live {
		t.Error("the override is still live after it was revoked")
	}

	after := e.decide(t, req)
	if after.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s) after the override was revoked, want a refusal again", after.ReasonCode)
	}
	if after.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q: the original refusal must stand once more",
			after.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}

// TestAdminRevokesAnAdminOverride is the case CanOverrule refused and CanRevoke
// admits: admin is the top human authority, so an admin-recorded vouch has no
// higher seat to take it back — leaving it revocable only by admin, or by
// nobody at all. Mirrors TestAdminRevokesAUserOverride, recorded one tier up.
func TestAdminRevokesAnAdminOverride(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantOverride(t, e, e.contact, string(commsauthz.LevelAdmin))

	adminCtx := writerCtxAt(e.ws, ids.NewV7(), "admin")
	if err := e.store.RevokeOverride(adminCtx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: row, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("an admin revoking an admin's override: %v", err)
	}
	if overrideStillLive(t, e, row) {
		t.Error("an admin-recorded override is still live after an admin revoked it")
	}
}

// TestAUserCannotRevokeAnAdminOverride is the laundering guard's non-merge twin:
// a rep never reaches an admin-recorded row, so the only way an admin vouch
// comes back is an admin taking it back. CanRevoke adds admin-revokes-admin
// without lowering the floor for a rep.
func TestAUserCannotRevokeAnAdminOverride(t *testing.T) {
	e := setupChannelConsent(t)
	own := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}
	row := plantOverride(t, e, own, string(commsauthz.LevelAdmin))

	err := e.store.RevokeOverride(boundedRepCtx(e.ws, e.user), RevokeOverrideInput{
		ContactID: own, OverrideID: row, Reason: "I disagree with the admin's vouch",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep revoking an admin's override answered %v, want ErrPermissionDenied", err)
	}
	if !overrideStillLive(t, e, row) {
		t.Error("a refused revoke took back the admin's row anyway")
	}
}

// plantOverride writes a row at a named level, bypassing the write door so a
// revoke test can start from a known level without depending on Allow's own
// behaviour — the same reason plantSuppression bypasses Suppress for the lift
// tests.
func plantOverride(t *testing.T, e *channelConsentEnv, contact ids.ContactID, level string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_override
		    (id, contact_id, category, reason, decided_by_level, captured_by)
		VALUES ($1, $2, 'marketing', 'the lead confirmed by phone', $3, 'human:x')`,
		id, contact, level); err != nil {
		t.Fatalf("planting a %s-level override: %v", level, err)
	}
	return id
}

// overrideStillLive reports whether the row is unrevoked.
func overrideStillLive(t *testing.T, e *channelConsentEnv, id ids.UUID) bool {
	t.Helper()
	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at IS NULL FROM communication_override WHERE id = $1`, id).Scan(&live); err != nil {
		t.Fatalf("reading the override back: %v", err)
	}
	return live
}

// TestAUserCannotRevokeAnotherUsersOverride holds the equal-rank arm, the same
// shape TestARepDoesNotLiftAnotherRepsStop holds for a stop: two reps
// disagreeing about a contact is a real situation, and reversing a peer's
// vouch stays a deliberate escalation rather than something a rep does by
// pressing a button.
func TestAUserCannotRevokeAnotherUsersOverride(t *testing.T) {
	e := setupChannelConsent(t)
	own := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Their Own Contact', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}
	row := plantOverride(t, e, own, string(commsauthz.LevelUser))

	err := e.store.RevokeOverride(boundedRepCtx(e.ws, e.user), RevokeOverrideInput{
		ContactID: own, OverrideID: row, Reason: "I disagree with the vouch",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep revoking a peer's override answered %v, want ErrPermissionDenied", err)
	}
	if !overrideStillLive(t, e, row) {
		t.Error("a refused revoke took back the row anyway")
	}
}

// TestRevokeUnknownOverrideIsNotFound holds the same non-disclosure lift.go's
// equivalent test does: a row that never existed answers exactly like one
// this caller was never going to be allowed to touch.
func TestRevokeUnknownOverrideIsNotFound(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: ids.NewV7(), Reason: "does not exist",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("revoking an unknown override answered %v, want ErrNotFound", err)
	}
}

// TestRevokeRequiresAReason is the same asymmetry requireReason states for a
// lift: a vouch that gets taken back is the write most worth being able to
// explain later, so the reason is mandatory rather than merely bounded.
func TestRevokeRequiresAReason(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantOverride(t, e, e.contact, string(commsauthz.LevelUser))

	err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{ContactID: e.contact, OverrideID: row})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("an empty reason was refused with %v, want a validation error", err)
	}
	if invalid.Field != fieldReason {
		t.Errorf("refused on field %q, want %q", invalid.Field, fieldReason)
	}
	if !overrideStillLive(t, e, row) {
		t.Error("a refused reason revoked the row anyway")
	}
}

// lastRecordedOverridePayload decodes the consent.override_recorded envelope the
// write just staged. Read from the outbox rather than returned by the store,
// because the row a consumer will actually receive is the thing under test —
// the same reason lastLiftPayload reads it there.
func lastRecordedOverridePayload(
	t *testing.T, e *channelConsentEnv,
) crmcontracts.PublicEventConsentOverrideRecorded {
	t.Helper()
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `
		SELECT envelope->'payload' FROM event_outbox
		 WHERE envelope->>'type' = 'consent.override_recorded'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("reading the staged override event: %v", err)
	}
	var payload crmcontracts.PublicEventConsentOverrideRecorded
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decoding the override payload: %v", err)
	}
	return payload
}

// TestTheDoorAndItsEventNameTheRowThatWasWritten is why the id is returned and
// on the payload at all.
//
// No endpoint lists a contact's standing overrides, so these two are the ONLY
// places the id the revoke door takes in its path is ever disclosed: the answer
// to the caller who recorded it, and the event to a consumer holding a
// subscription. Either one naming the category but not the row leaves a rep able
// to record a vouch and unable to take it back — and, after a merge has left two
// live rows for one category, unable to tell which of them any later
// consent.override_lifted described.
//
// Both are asserted against the row actually in the table, so a door returning a
// plausible id that is not the one it wrote fails here rather than at a revoke
// weeks later.
func TestTheDoorAndItsEventNameTheRowThatWasWritten(t *testing.T) {
	e := setupChannelConsent(t)

	returned, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	var written ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id FROM communication_override
		 WHERE contact_id = $1 AND revoked_at IS NULL`, e.contact).Scan(&written); err != nil {
		t.Fatalf("reading back the override id: %v", err)
	}
	if returned != written {
		t.Errorf("the door returned override %s, want the row it wrote, %s", returned, written)
	}

	payload := lastRecordedOverridePayload(t, e)
	if ids.UUID(payload.OverrideId) != written {
		t.Errorf("the event named override %s, want the row that was written, %s",
			ids.UUID(payload.OverrideId), written)
	}
	if payload.Category != "marketing" {
		t.Errorf("the event named category %q, want marketing", payload.Category)
	}
}

// TestTheReturnedIdIsWhatTheRevokeDoorTakes closes the loop the two disclosures
// exist for: the id the door hands back is accepted by RevokeOverride with no
// other lookup in between. Asserted rather than assumed, because "an id was
// returned" and "that id revokes the vouch" are different claims, and only the
// second is the reason the response body exists.
func TestTheReturnedIdIsWhatTheRevokeDoorTakes(t *testing.T) {
	e := setupChannelConsent(t)

	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("revoking the override the door just named: %v", err)
	}
	if overrideStillLive(t, e, recorded) {
		t.Error("the override the door named is still live after it was revoked")
	}
}

// TestARevokeThroughAMergedAwayContactReachesTheRowAllowMovedToTheSurvivor
// closes the pair of doors over each other. Allow settles its subject against
// a merge and writes the row on the survivor, then answers with that row's id
// so the caller can take it back. A revoke that pinned the row to the contact
// in the URL would answer 404 to the very id it just handed out.
func TestARevokeThroughAMergedAwayContactReachesTheRowAllowMovedToTheSurvivor(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Merge Survivor', 'test', 'human:x', 'workspace', $2)`, survivor, e.user); err != nil {
		t.Fatal(err)
	}
	// The merge's one durable mark, written the way mergeContactTx leaves it.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET merged_into_id = $1 WHERE id = $2`, survivor, e.contact); err != nil {
		t.Fatal(err)
	}

	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "confirmed on a call after the merge",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	// The premise: Allow settled the row onto the survivor, not the named contact.
	if category, _, _ := liveOverrideRow(t, e, survivor); category != "marketing" {
		t.Fatalf("the row on the survivor is for %q, want marketing", category)
	}

	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "the buyer withdrew it",
	}); err != nil {
		t.Fatalf("revoking through the contact the vouch was recorded through: %v", err)
	}

	var live int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_override
		 WHERE contact_id = $1 AND revoked_at IS NULL`, survivor).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("the survivor still holds %d live override(s), want 0", live)
	}
}

// boundedAdminCtx is boundedRepCtx's seat with the admin role: its authority
// level can take back an admin's vouch, while its row scope still stops at the
// records it owns. A rep-level seat could not exercise the survivor check at
// all, because the level comparison refuses it first.
func boundedAdminCtx(ws, user ids.UUID) context.Context {
	ctx := boundedRepCtx(ws, user)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RoleKeys = []string{"admin"}
	return principal.WithActor(ctx, actor)
}

// seedMergedPair seeds a contact the rep owns, merged into a survivor somebody
// else owns, so a bounded seat may write the retired record and not the one it
// settles onto.
func seedMergedPair(t *testing.T, e *channelConsentEnv) (own, other ids.ContactID) {
	t.Helper()
	own = ids.New[ids.ContactKind]()
	other = ids.New[ids.ContactKind]()
	elsewhere := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other Rep')`,
		elsewhere, "rep-"+elsewhere.String()+"@cc.test"); err != nil {
		t.Fatalf("seeding the survivor's owner: %v", err)
	}
	for _, row := range []struct {
		id    ids.ContactID
		name  string
		owner ids.UUID
	}{{own, "Rep Owned Source", e.user}, {other, "Somebody Elses Survivor", elsewhere}} {
		if _, err := e.owner.Exec(context.Background(), `
			INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
			VALUES ($1, $2, 'test', 'human:x', 'workspace', $3)`, row.id, row.name, row.owner); err != nil {
			t.Fatalf("seeding %s: %v", row.name, err)
		}
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET merged_into_id = $1 WHERE id = $2`, other, own); err != nil {
		t.Fatalf("merging the pair: %v", err)
	}
	return own, other
}

// TestARevokeThroughAMergedAwayContactCannotReachTheSurvivorsOwnVouch holds
// the widened lookup to the caller's reach. Accepting a row on the survivor is
// what lets a pre-merge handle work; it must not let a seat that may write the
// retired record take back a vouch somebody recorded on a survivor that seat
// may not touch.
func TestARevokeThroughAMergedAwayContactCannotReachTheSurvivorsOwnVouch(t *testing.T) {
	e := setupChannelConsent(t)
	own, other := seedMergedPair(t, e)
	theirs, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: other, Category: "marketing", Reason: "their account manager confirmed it",
	})
	if err != nil {
		t.Fatalf("recording the survivor's own override: %v", err)
	}

	err = e.store.RevokeOverride(boundedAdminCtx(e.ws, e.user), RevokeOverrideInput{
		ContactID: own, OverrideID: theirs, Reason: "trying through the record I own",
	})
	// ErrPermissionDenied, the write-authority arm: the survivor has workspace
	// visibility, so this seat can see it and the scope arm admits it; the
	// owner check is what refuses. The seat is an admin so the level
	// comparison passes and only the survivor's scope is on trial.
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want %v: the revoke reached a vouch on a survivor this seat may not write", err, apperrors.ErrPermissionDenied)
	}
	if !overrideStillLive(t, e, theirs) {
		t.Error("the survivor's own vouch was taken back by a seat that may not write the survivor")
	}
}

// TestARevokeNamesTheRowsSubjectOrItsSurvivorAndNothingElse pins the other
// edge of the widened lookup: a row whose chain never touches the contact
// named or its survivor stays unreachable and unrevealed.
func TestARevokeNamesTheRowsSubjectOrItsSurvivorAndNothingElse(t *testing.T) {
	e := setupChannelConsent(t)
	stranger := seedOverrideContact(t, e, "Unrelated Contact")
	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: stranger, Category: "marketing", Reason: "confirmed on a call",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}

	err = e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "not mine to take back",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found: a row on an unrelated contact must stay hidden", err)
	}
}

// TestAllowThroughAMergedAwayContactChecksTheSurvivorItWritesOnto is the
// write side of the same reach: the row lands on the survivor, so the
// survivor's scope is read before it does.
func TestAllowThroughAMergedAwayContactChecksTheSurvivorItWritesOnto(t *testing.T) {
	e := setupChannelConsent(t)
	own, other := seedMergedPair(t, e)

	_, err := e.store.Allow(boundedRepCtx(e.ws, e.user), AllowInput{
		ContactID: own, Category: "marketing", Reason: "vouching through the record I own",
	})
	// ErrPermissionDenied, the write-authority arm, for the reason the revoke
	// test above gives.
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want %v: the vouch landed on a survivor this seat may not write", err, apperrors.ErrPermissionDenied)
	}
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_override WHERE contact_id = $1`, other).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the survivor holds %d override(s), want 0", n)
	}
}

// TestARevokeThroughTheLiveSurvivorReachesTheOriginalItWasCopiedFrom is the
// handle a rep actually holds after a merge: the id Allow answered, and the
// contact the UI still shows. The original sits on the retired record and the
// copy on the survivor; the chain is one vouch, so naming the survivor with the
// original's id takes both back.
func TestARevokeThroughTheLiveSurvivorReachesTheOriginalItWasCopiedFrom(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedOverrideContact(t, e, "Live Survivor")
	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	carryOverrides(t, e, e.contact, survivor)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET merged_into_id = $1 WHERE id = $2`, survivor, e.contact); err != nil {
		t.Fatal(err)
	}

	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: survivor, OverrideID: recorded, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("revoking through the survivor with the original's id: %v", err)
	}
	for _, c := range []ids.ContactID{e.contact, survivor} {
		if n := liveOverridesOn(t, e, c); n != 0 {
			t.Errorf("contact %s still holds %d live override(s), want 0", c.UUID, n)
		}
	}
}

// TestTheAllowDoorAnswers422ForACategoryItCannotResolve holds the wire half of
// the validation contract: a category outside the engine's vocabulary is the
// caller's fault, and the documented answer is 422, not a server error.
func TestTheAllowDoorAnswers422ForACategoryItCannotResolve(t *testing.T) {
	e := setupChannelConsent(t)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"category":"not-a-category","reason":"they asked us at the trade fair"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts/x/consent/allow", body).WithContext(e.ctx)
	req.Header.Set("Content-Type", "application/json")

	Handlers{store: e.store}.AllowContact(rec, req, crmcontracts.Id(e.contact.UUID))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the allow door answered %d: %s, want 422", rec.Code, rec.Body.String())
	}
}

// TestTheRevokeDoorAnswers422ForAnEmptyReason holds the second door's half of
// the validation contract: a missing reason is the caller's fault, answered 422.
func TestTheRevokeDoorAnswers422ForAnEmptyReason(t *testing.T) {
	e := setupChannelConsent(t)
	row := plantOverride(t, e, e.contact, string(commsauthz.LevelUser))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts/x/consent/allow/y/revoke",
		strings.NewReader(`{"reason":""}`)).WithContext(e.ctx)
	req.Header.Set("Content-Type", "application/json")

	Handlers{store: e.store}.RevokeOverride(rec, req, crmcontracts.Id(e.contact.UUID), openapi_types.UUID(row))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the revoke door answered %d: %s, want 422", rec.Code, rec.Body.String())
	}
	if !overrideStillLive(t, e, row) {
		t.Error("a refused reason revoked the row anyway")
	}
}
