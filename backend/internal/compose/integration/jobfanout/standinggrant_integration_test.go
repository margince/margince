// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

// A rep's standing authority, over real migrated Postgres.
//
// The subject is the state machine, because it IS the security boundary. The
// grant records a decision and never confers authority: what lets an agent act
// is a passport, and there is exactly one production statement that mints one —
// it binds on_behalf_of and granted_by to the same session user, so a rep can
// only ever be acted for by a credential they minted themselves.
//
// These tests seed through that real endpoint rather than inserting a passport
// row, because a hand-inserted credential proves nothing about the path a rep
// actually takes.

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const grantSpec = "morning_brief"

// mintPassportFor takes the path a rep takes: POST /v1/passports on their own
// session. There is no other way to obtain one, which is the point.
func (re *runnerEnv) mintPassportFor(t *testing.T) ids.PassportID {
	t.Helper()
	var minted struct {
		PassportID string `json:"passport_id"`
	}
	if status := re.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		"label": "overnight brief", "scopes": []string{"read"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	id, err := ids.ParseAs[ids.PassportKind](minted.PassportID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// sessionUser is the rep whose session the harness is signed in as — the one
// the minted passport is bound to.
func (re *runnerEnv) sessionUser(t *testing.T) ids.UserID {
	t.Helper()
	var raw string
	if err := re.Owner.QueryRow(context.Background(),
		`SELECT on_behalf_of::text FROM passport ORDER BY created_at DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("no passport to read the session user from: %v", err)
	}
	id, err := ids.ParseAs[ids.UserKind](raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// mintPassportForColleague makes a passport that acts as somebody other than
// the session user.
//
// It inserts directly, which is deliberate and is the ONLY place this suite
// does so: there is no product path that mints a passport for another person —
// that is the invariant — so a test needing one has to write the row the
// product refuses to write, in order to prove what happens if it existed.
func (re *runnerEnv) mintPassportForColleague(t *testing.T, colleague ids.UserID) ids.PassportID {
	t.Helper()
	var raw string
	if err := re.Owner.QueryRow(context.Background(), `
		INSERT INTO passport (on_behalf_of, granted_by, scopes, token_hash, expires_at)
		VALUES ($1, $1, ARRAY['read'], $2, now() + interval '30 days')
		RETURNING id::text`, colleague, "probe-"+ids.NewV7().String()).Scan(&raw); err != nil {
		t.Fatalf("seed a colleague's passport: %v", err)
	}
	id, err := ids.ParseAs[ids.PassportKind](raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// colleague is a rep who is NOT the session user. The decline and never-asked
// cases need one: they turn on the absence of a credential, and the session
// user's passport would confuse what the test is actually asserting.
func (re *runnerEnv) colleague(t *testing.T, email string) ids.UserID {
	t.Helper()
	var raw string
	if err := re.Owner.QueryRow(context.Background(),
		`INSERT INTO app_user (email, display_name) VALUES ($1, $2) RETURNING id::text`,
		email, "Colleague "+email).Scan(&raw); err != nil {
		t.Fatalf("seed a colleague: %v", err)
	}
	id, err := ids.ParseAs[ids.UserKind](raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// asRep binds a context to one rep, the way a signed-in session does. MyGrant
// reads the acting principal rather than taking a user id, so this is how a
// test asks for a particular rep's own row.
func (re *runnerEnv) asRep(user ids.UserID) context.Context {
	return principal.WithActor(re.wsCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: user.String(), UserID: user.UUID,
	})
}

// recordDecision answers AS the given rep. RecordDecisionTx takes no user id —
// it reads the acting principal — so the rep is expressed by the context, which
// is the same thing a signed-in session does.
func (re *runnerEnv) recordDecision(
	t *testing.T, user ids.UserID, state string, passport *ids.PassportID,
) error {
	t.Helper()
	ctx := re.asRep(user)
	db := database.BindTo(re.pool, ids.From[ids.WorkspaceKind](re.wsID))
	return db.Tx(ctx, func(tx pgx.Tx) error {
		return runner.RecordDecisionTx(ctx, tx, grantSpec, state, passport)
	})
}

// A rep who said yes is enumerable by the nightly fan-out.
func TestAGrantedRepIsFoundByTheNightlyFanOut(t *testing.T) {
	e := setupRunner(t)
	passport := e.mintPassportFor(t)
	rep := e.sessionUser(t)
	if err := e.recordDecision(t, rep, runner.GrantStateGranted, &passport); err != nil {
		t.Fatalf("record the grant: %v", err)
	}

	live, err := e.store.LiveGrantsFor(e.wsCtx, grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Fatalf("live grants = %d, want 1", len(live))
	}
	if live[0].UserID != rep {
		t.Errorf("the fan-out found user %s, want the granting rep %s", live[0].UserID, rep)
	}
	if live[0].PassportID == nil || *live[0].PassportID != passport {
		t.Errorf("the grant names passport %v, want the one the rep minted %s",
			live[0].PassportID, passport)
	}
	if !live[0].Live() {
		t.Error("a granted rep with a live passport does not read as live")
	}
}

// A rep who said no is remembered, and is not asked again.
func TestADecliningRepIsRememberedAndNotEnumerated(t *testing.T) {
	e := setupRunner(t)
	rep := e.colleague(t, "declines@grant.test")
	if err := e.recordDecision(t, rep, runner.GrantStateDeclined, nil); err != nil {
		t.Fatalf("record the decline: %v", err)
	}

	// Remembered: the product can tell "said no" from "never asked", which is
	// the whole reason the row exists. Without it the rep is asked every night.
	grant, found, err := e.store.MyGrant(e.asRep(rep), grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("the decline was not recorded — the rep will be asked again tomorrow")
	}
	if grant.State != runner.GrantStateDeclined {
		t.Errorf("state = %q, want declined", grant.State)
	}
	if grant.Live() {
		t.Error("a declined grant reads as live authority")
	}

	live, err := e.store.LiveGrantsFor(e.wsCtx, grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range live {
		if g.UserID == rep {
			t.Error("the fan-out enumerated a rep who declined")
		}
	}
}

// A rep nobody asked has no row, and that is not a failure.
func TestARepWhoWasNeverAskedHasNoGrant(t *testing.T) {
	e := setupRunner(t)
	_, found, err := e.store.MyGrant(e.asRep(e.colleague(t, "unasked@grant.test")), grantSpec)
	if err != nil {
		t.Fatalf("reading an unasked rep's grant failed: %v — the first visit is "+
			"the ordinary case and must not look like a fault", err)
	}
	if found {
		t.Error("a rep nobody asked already has an answer recorded")
	}
}

// One rep cannot read another's decision, and the bound is the signature.
//
// MyGrant takes no user id: it reads the acting principal and selects on it, so
// there is no argument a handler could pass to reach a colleague's row. This
// pins that — an exported read taking a user id would make the cross-rep call
// expressible, and then only a reviewer stands between it and a caller.
func TestARepReadsTheirOwnDecisionAndNobodyElses(t *testing.T) {
	e := setupRunner(t)
	passport := e.mintPassportFor(t)
	mine := e.sessionUser(t)
	if err := e.recordDecision(t, mine, runner.GrantStateGranted, &passport); err != nil {
		t.Fatal(err)
	}
	colleague := e.colleague(t, "othergrant@grant.test")
	if err := e.recordDecision(t, colleague, runner.GrantStateDeclined, nil); err != nil {
		t.Fatal(err)
	}

	// Each rep's own context answers with their own row, and there is no third
	// call shape that could answer with somebody else's.
	granted, found, err := e.store.MyGrant(e.asRep(mine), grantSpec)
	if err != nil || !found {
		t.Fatalf("the granting rep cannot read their own decision (found=%v, err=%v)", found, err)
	}
	if !granted.Live() {
		t.Errorf("state = %q, want the rep's own granted answer", granted.State)
	}
	declined, found, err := e.store.MyGrant(e.asRep(colleague), grantSpec)
	if err != nil || !found {
		t.Fatalf("the declining rep cannot read their own decision (found=%v, err=%v)", found, err)
	}
	if declined.State != runner.GrantStateDeclined {
		t.Errorf("the colleague's own read answered %q, want their own declined answer", declined.State)
	}
}

// A principal with no human behind it is refused, not answered.
//
// Answering "never asked" would invite the caller to offer it the confirmation,
// and a machine principal has no standing decision of its own to make.
func TestAPrincipalWithNoHumanHasNoStandingGrantToRead(t *testing.T) {
	e := setupRunner(t)
	if _, _, err := e.store.MyGrant(e.wsCtx, grantSpec); err == nil {
		t.Fatal("a principal with no human behind it was answered — it has no " +
			"decision of its own, and answering invites the caller to offer it one")
	}
}

// A grant may not name somebody else's credential.
//
// This is the one way the "nobody is acted for by a credential they did not
// mint" invariant can still be broken, and it is not the mint: the passport
// itself is minted correctly, for its own owner. What breaks it is a GRANT row
// pairing one rep's user id with another rep's passport, because the fan-out
// then runs for the named rep under the named credential — acting as the
// passport's owner, on the say-so of somebody who never asked them.
func TestAGrantCannotNameACredentialBelongingToSomebodyElse(t *testing.T) {
	e := setupRunner(t)
	// A passport minted correctly, by and for the session user.
	mine := e.mintPassportFor(t)
	owner := e.sessionUser(t)
	colleague := e.colleague(t, "borrowed@grant.test")
	if colleague == owner {
		t.Fatal("the fixture handed back the same user twice; this proves nothing")
	}

	// The pairing that must not exist, either way round: a grant whose user and
	// whose passport are different people. The fan-out reads (user, passport)
	// and authenticates the passport, so the run does what the PASSPORT's owner
	// may do while the grant says whose it is.
	if err := e.recordDecision(t, colleague, runner.GrantStateGranted, &mine); err == nil {
		t.Fatal("a grant was recorded pairing one rep with another rep's passport — " +
			"the fan-out would run for them under a credential they never minted")
	}
	// And the reverse pairing, so neither rep can borrow from the other
	// whichever way round the row is written.
	theirs := e.mintPassportForColleague(t, colleague)
	if err := e.recordDecision(t, owner, runner.GrantStateGranted, &theirs); err == nil {
		t.Fatal("a rep recorded their own grant naming a colleague's passport")
	}
}

// A machine principal cannot record a standing decision.
//
// The rep is read from the acting principal rather than passed in, so the only
// answer any caller can record is their own. A principal with no human behind
// it has no answer to give, and letting it write one would make "the system
// turned it on for you" expressible — which is the whole thing this feature
// exists not to be.
func TestAPrincipalWithNoHumanCannotRecordAStandingDecision(t *testing.T) {
	e := setupRunner(t)
	passport := e.mintPassportFor(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.wsID))
	// e.wsCtx carries the scheduler's own system actor, no human behind it.
	err := db.Tx(e.wsCtx, func(tx pgx.Tx) error {
		return runner.RecordDecisionTx(e.wsCtx, tx, grantSpec, runner.GrantStateGranted, &passport)
	})
	if err == nil {
		t.Fatal("a machine principal recorded a standing grant — nobody's authority " +
			"may be granted by something that is not them")
	}
	// Two guards refuse this independently: the human check, and the ownership
	// check, since a principal with no user id can own no passport. Either
	// alone is enough, which is why removing one does not make this test pass
	// for the wrong reason.
	var rows int
	if qErr := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_standing_grant`).Scan(&rows); qErr != nil {
		t.Fatal(qErr)
	}
	if rows != 0 {
		t.Errorf("grants on file = %d, want 0 — the refusal did not stop the write", rows)
	}
}

// A granted row may never exist without the credential it claims.
//
// This is the shape the whole design must not reach: it reads as standing
// authority to anything checking the state, while carrying nothing to act
// with. The database refuses it, and so does the writer.
func TestAGrantCannotClaimAuthorityItHasNoCredentialFor(t *testing.T) {
	e := setupRunner(t)
	rep := e.colleague(t, "nocredential@grant.test")
	if err := e.recordDecision(t, rep, runner.GrantStateGranted, nil); err == nil {
		t.Fatal("a granted row with no passport was accepted — it claims an " +
			"authority nothing can perform")
	}

	// And the constraint holds independently of the writer, so a future caller
	// that bypasses RecordDecisionTx cannot create one either.
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.wsID))
	err := db.Tx(e.wsCtx, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.wsCtx, `
			INSERT INTO agent_standing_grant (user_id, agent_spec, state, passport_id)
			VALUES ($1, $2, 'granted', NULL)`, rep, grantSpec)
		return execErr
	})
	if err == nil {
		t.Error("the database accepted a granted grant with no passport")
	}
}

// A revoked credential ends the authority and keeps the decision.
//
// The rep agreed once, and a credential dying does not un-agree it — what they
// need next is the renewal question, not the first-time one. Getting that wrong
// reads as the product having forgotten them.
func TestARevokedPassportEndsTheAuthorityAndKeepsTheDecision(t *testing.T) {
	e := setupRunner(t)
	passport := e.mintPassportFor(t)
	rep := e.sessionUser(t)
	if err := e.recordDecision(t, rep, runner.GrantStateGranted, &passport); err != nil {
		t.Fatal(err)
	}
	// Revoked the way the product revokes: the row's own soft state.
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE passport SET revoked_at = now() WHERE id = $1`, passport); err != nil {
		t.Fatal(err)
	}

	grant, found, err := e.store.MyGrant(e.asRep(rep), grantSpec)
	if err != nil || !found {
		t.Fatalf("the grant disappeared with the credential (found=%v, err=%v) — "+
			"the rep would be asked as though for the first time", found, err)
	}
	if !grant.NeedsRenewal() {
		t.Error("the rep is not offered a renewal — they agreed once, and the " +
			"first-time question reads as the product having forgotten")
	}
	if grant.Live() {
		t.Error("a revoked credential still reads as live authority")
	}
	if grant.State != runner.GrantStateGranted {
		t.Errorf("state = %q, want the rep's answer to stand — the credential "+
			"died, the decision did not", grant.State)
	}

	live, err := e.store.LiveGrantsFor(e.wsCtx, grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range live {
		if g.UserID == rep {
			t.Error("the fan-out enumerated a rep whose credential is revoked — the " +
				"run it queues can only fail")
		}
	}
}

// An EXPIRED passport is not enumerated either, though the row still names it.
//
// Expiry is not an event anything observes: the passport simply stops being
// usable at a moment nothing writes a row for. A fan-out trusting the grant
// state alone would queue a night's work that fails at claim time, and the rep
// sees a broken morning instead of the honest "your authority lapsed".
func TestAnExpiredPassportIsNotEnumeratedEvenThoughTheGrantStands(t *testing.T) {
	e := setupRunner(t)
	passport := e.mintPassportFor(t)
	rep := e.sessionUser(t)
	if err := e.recordDecision(t, rep, runner.GrantStateGranted, &passport); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE passport SET expires_at = now() - interval '1 day' WHERE id = $1`, passport); err != nil {
		t.Fatal(err)
	}

	live, err := e.store.LiveGrantsFor(e.wsCtx, grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range live {
		if g.UserID == rep {
			t.Error("the fan-out enumerated a rep whose passport has expired")
		}
	}
	// And the rep's own card says the same thing, from the same join: the two
	// surfaces cannot disagree about whether their authority still works.
	grant, _, err := e.store.MyGrant(e.asRep(rep), grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !grant.NeedsRenewal() {
		t.Error("the rep's own card does not offer a renewal for an expired credential")
	}
}

// Changing your mind replaces the answer; it does not add a second one.
func TestARepWhoChangesTheirMindHasOneAnswer(t *testing.T) {
	e := setupRunner(t)
	rep := e.sessionUser(t)
	if err := e.recordDecision(t, rep, runner.GrantStateDeclined, nil); err != nil {
		t.Fatal(err)
	}
	passport := e.mintPassportFor(t)
	if err := e.recordDecision(t, rep, runner.GrantStateGranted, &passport); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_standing_grant WHERE user_id = $1 AND agent_spec = $2`,
		rep, grantSpec).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("answers on file = %d, want 1 — two rows let the product read "+
			"whichever it finds first", rows)
	}
	grant, _, err := e.store.MyGrant(e.asRep(rep), grantSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !grant.Live() {
		t.Errorf("state = %q, want the newer answer to stand", grant.State)
	}
}
