// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestARecordedRequestServesItsSendingEntityOnly(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		branch bool
	}{{"default", false}, {"branch", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			e := setupResolve(t)
			branch := e.secondEntity(t)
			recordedEntity, otherEntity := ids.Nil, branch
			if scenario.branch {
				recordedEntity, otherEntity = branch, ids.Nil
			}
			recorder := e.colleague(t, recordedEntity)
			if _, err := e.store.RecordQualifyingEvent(e.seatCtx(recorder), e.contact, RecordQualifyingEventInput{
				Kind: KindRequestedBySubject, Note: "Asked this company to follow up by email.",
				OccurredAt: time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC),
			}); err != nil {
				t.Fatal(err)
			}
			same := e.colleague(t, recordedEntity)
			other := e.colleague(t, otherEntity)
			if got := e.decideRequestAs(t, same, ids.Nil); got.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("a colleague of the same entity got %s (%s), want allow", got.Verdict, got.ReasonCode)
			}
			if got := e.decideRequestAs(t, other, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
				t.Fatalf("a different sending entity used the request: %s", got.ReasonCode)
			}
			originalEntity := recordedEntity
			if originalEntity.IsZero() {
				if err := e.owner.QueryRow(t.Context(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&originalEntity); err != nil {
					t.Fatal(err)
				}
			}
			if got := e.decideRequestAs(t, other, originalEntity); got.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("a queued message lost its original sending entity: %s", got.ReasonCode)
			}
			if _, err := e.owner.Exec(t.Context(), `DELETE FROM legal_entity_member WHERE user_id=@recorder`, pgx.StrictNamedArgs{"recorder": recorder}); err != nil {
				t.Fatal(err)
			}
			if !otherEntity.IsZero() {
				if _, err := e.owner.Exec(t.Context(), `INSERT INTO legal_entity_member (user_id,legal_entity_id,assigned_by) VALUES (@user,@entity,'human:x')`, pgx.StrictNamedArgs{"user": recorder, "entity": otherEntity}); err != nil {
					t.Fatal(err)
				}
			}
			if got := e.decideRequestAs(t, same, ids.Nil); got.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("the original entity lost its request after the recorder moved: %s", got.ReasonCode)
			}
			if got := e.decideRequestAs(t, recorder, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
				t.Fatal("the recorder carried the request to their new entity")
			}
		})
	}
}

func (e *resolveEnv) decideRequestAs(t *testing.T, user, entity ids.UUID) commsauthz.Decision {
	t.Helper()
	ctx := e.seatCtx(user)
	var decision commsauthz.Decision
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		req, err := withSendingEntity(ctx, tx, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, SendingEntity: entity})
		if err != nil {
			return err
		}
		decision, err = e.gate.decideOne(ctx, tx, connector.Recipient{Email: e.address}, req, commsauthz.PhaseStaging)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return decision
}
