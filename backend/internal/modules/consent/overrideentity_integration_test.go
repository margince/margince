// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Row 11 of the approved send rules. A standing vouch serves every rep of the
// legal entity whose rep recorded it. It does not lapse for age or an owner
// change. It never reaches a send for another entity.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// colleague seats a further rep in the env, working for entity when it is set.
func (e *resolveEnv) colleague(t *testing.T, entity ids.UUID) ids.UUID {
	t.Helper()
	user := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		user, "colleague-"+user.String()+"@r.test"); err != nil {
		t.Fatalf("seating the colleague: %v", err)
	}
	if !entity.IsZero() {
		if _, err := e.owner.Exec(context.Background(), `
			INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by)
			VALUES ($1, $2, 'human:x')`, entity, user); err != nil {
			t.Fatalf("assigning the colleague to the entity: %v", err)
		}
	}
	return user
}

// secondEntity describes a further legal entity of the installation.
func (e *resolveEnv) secondEntity(t *testing.T) ids.UUID {
	t.Helper()
	entity := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by)
		VALUES ($1, 'Branch', 'Branch Ltd', 'at', 'human:x')`, entity); err != nil {
		t.Fatalf("describing the second entity: %v", err)
	}
	return entity
}

// vouchAs records a marketing vouch through the real door, as user.
func (e *resolveEnv) vouchAs(t *testing.T, user ids.UUID) ids.UUID {
	t.Helper()
	id, err := e.store.Allow(e.seatCtx(user), AllowInput{
		ContactID: e.contact, Category: string(commsauthz.CategoryMarketing),
		Reason: "she asked on the call for the spring catalogue",
	})
	if err != nil {
		t.Fatalf("recording the vouch: %v", err)
	}
	return id
}

// decideAs runs the whole per-recipient decision as user, naming the sending
// entity as staging does.
func (e *resolveEnv) decideAs(t *testing.T, user ids.UUID) commsauthz.Decision {
	t.Helper()
	ctx := e.seatCtx(user)
	var out commsauthz.Decision
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		req, err := withSendingEntity(ctx, tx, commsauthz.Request{LegacyPurposeKey: "newsletter"})
		if err != nil {
			return err
		}
		out, err = e.gate.decideOne(ctx, tx, connector.Recipient{Email: e.address}, req, commsauthz.PhaseStaging)
		return err
	}); err != nil {
		t.Fatalf("deciding: %v", err)
	}
	return out
}

func TestAVouchServesEveryRepOfItsEntityAndNoOther(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	branch := e.secondEntity(t)
	recorder := e.colleague(t, ids.Nil)
	sameEntity := e.colleague(t, ids.Nil)
	otherEntity := e.colleague(t, branch)

	vouch := e.vouchAs(t, recorder)

	if got := e.decideAs(t, sameEntity); got.Verdict != commsauthz.VerdictAllow || got.OverrideID != vouch {
		t.Fatalf("a colleague of the same entity got %q (%s, override %s), want allow on vouch %s",
			got.Verdict, got.ReasonCode, got.OverrideID, vouch)
	}
	got := e.decideAs(t, otherEntity)
	if got.Verdict == commsauthz.VerdictAllow || !got.OverrideID.IsZero() {
		t.Fatalf("a rep of another entity got %q (override %s), want the refusal: a vouch never "+
			"crosses legal entities", got.Verdict, got.OverrideID)
	}
	if got.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q", got.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}

func TestAVouchRecordedForAnotherEntityStaysWithIt(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	branch := e.secondEntity(t)
	vouch := e.vouchAs(t, e.colleague(t, branch))

	if got := e.decideAs(t, e.colleague(t, branch)); got.OverrideID != vouch {
		t.Fatalf("a colleague in the branch got override %s, want %s", got.OverrideID, vouch)
	}
	if got := e.decideAs(t, e.colleague(t, ids.Nil)); !got.OverrideID.IsZero() {
		t.Fatalf("a rep of the default entity sent on the branch's vouch %s", got.OverrideID)
	}
}

func TestAVouchDoesNotLapseForAgeOrAnOwnerChange(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	recorder := e.colleague(t, ids.Nil)
	vouch := e.vouchAs(t, recorder)

	threeYearsAgo := clocktest.Now(t).Add(-3 * 365 * 24 * time.Hour)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_override SET recorded_at = $2 WHERE id = $1`, vouch, threeYearsAgo); err != nil {
		t.Fatalf("ageing the vouch: %v", err)
	}
	newOwner := e.colleague(t, ids.Nil)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET owner_id = $2 WHERE id = $1`, e.contact, newOwner); err != nil {
		t.Fatalf("handing the contact over: %v", err)
	}

	if got := e.decideAs(t, newOwner); got.Verdict != commsauthz.VerdictAllow || got.OverrideID != vouch {
		t.Fatalf("the new owner got %q (%s), want allow on the three-year-old vouch %s",
			got.Verdict, got.ReasonCode, vouch)
	}
}
