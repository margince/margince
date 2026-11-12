// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Taking back a standing exception already on file, against a real database.
//
// Integration rather than unit: liveOverride reads the live rows inside the
// sending transaction. A revoke that left the row live is a defect no fake
// store could show.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

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
	e.livesIn(t, reviewableMarketingCountry())
	e.seedPurpose(t, "newsletter", "marketing")
	req := commsauthz.Request{LegacyPurposeKey: "newsletter"}

	planted := e.seedOverride(t, "marketing", false)

	before := e.decide(t, req)
	if before.Verdict != commsauthz.VerdictAllow || before.OverrideID != planted {
		t.Fatalf("verdict = %q (%s, override %s) with the exception on file, want allow on %s — "+
			"the fixture proves nothing otherwise", before.Verdict, before.ReasonCode, before.OverrideID, planted)
	}
	overrideID := before.OverrideID

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

// plantOverride writes a marketing exception at a named level, as one already
// on file. No door records one any more, so a row is the only way to start.
func plantOverride(t *testing.T, e *channelConsentEnv, contact ids.ContactID, level string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_override
		    (id, contact_id, category, reason, decided_by_level, captured_by, legal_entity_id, recorded_directly)
		VALUES ($1, $2, 'marketing', 'the lead confirmed by phone', $3, 'human:x', (SELECT id FROM legal_entity WHERE is_default), true)`,
		id, contact, level); err != nil {
		t.Fatalf("planting a %s-level override: %v", level, err)
	}
	return id
}

// recordedOverrideAs plants an exception in the shape a seat's own recording
// left: that seat's level, stamp and sending entity, recorded directly.
func recordedOverrideAs(ctx context.Context, t *testing.T, store *Store, contact ids.ContactID, category string) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := store.db.Tx(ctx, func(tx pgx.Tx) error {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		entity, err := sendingEntityOf(ctx, tx)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO communication_override
			    (contact_id, category, reason, decided_by_level, captured_by, legal_entity_id, recorded_directly)
			VALUES ($1, $2, 'the contact asked for it on a call', $3, $4, $5, true)
			RETURNING id`, contact, category, string(authorityOf(ctx)), by, entity).Scan(&id)
	})
	if err != nil {
		t.Fatalf("planting the recorded %s override: %v", category, err)
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

// TestARevokeThroughAMergedAwayContactReachesTheRowOnTheSurvivor: a caller
// still holding the retired contact reaches the exception on its survivor. A
// revoke that pinned the row to the contact in the URL would answer 404.
func TestARevokeThroughAMergedAwayContactReachesTheRowOnTheSurvivor(t *testing.T) {
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

	recorded := plantOverride(t, e, survivor, string(commsauthz.LevelAdmin))

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
	theirs := plantOverride(t, e, other, string(commsauthz.LevelAdmin))

	err := e.store.RevokeOverride(boundedAdminCtx(e.ws, e.user), RevokeOverrideInput{
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
	recorded := plantOverride(t, e, stranger, string(commsauthz.LevelAdmin))

	err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "not mine to take back",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found: a row on an unrelated contact must stay hidden", err)
	}
}

// TestARevokeThroughTheLiveSurvivorReachesTheOriginalItWasCopiedFrom: after a
// merge a rep may hold the original's id and the contact the UI still shows.
// The original sits on the retired record and the copy on the survivor. The
// chain is one vouch, so naming the survivor with the original's id takes both
// back.
func TestARevokeThroughTheLiveSurvivorReachesTheOriginalItWasCopiedFrom(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedOverrideContact(t, e, "Live Survivor")
	recorded := plantOverride(t, e, e.contact, string(commsauthz.LevelAdmin))
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
