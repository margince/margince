// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Four facts about stops. One kept as a hash still stops. A lead's stops go
// through the lift. A copy keeps how it was recorded. A merge binds a stop to
// its own record's addresses.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheSendCheckMatchesAStopKeptAsTheHashOfTheAddress(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (address_hash, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'kept', 'system', 'user')`,
		storekit.SuppressionHash("kept@returning.test")); err != nil {
		t.Fatalf("planting the kept stop: %v", err)
	}
	for _, c := range []struct {
		address string
		want    int
	}{
		{"Kept@Returning.test", 1},
		{"other@returning.test", 0},
	} {
		var stops []liveStop
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			var err error
			stops, err = liveSuppression(e.ctx, tx, ids.NewV7().String(), connector.Recipient{Email: c.address})
			return err
		}); err != nil {
			t.Fatalf("reading the stops: %v", err)
		}
		if len(stops) != c.want {
			t.Errorf("%s: %d stop(s), want %d", c.address, len(stops), c.want)
		}
	}
}

// A rep answering a refusal is told a kept stop already refuses the message,
// the same way the send check would.
//
// Mutation: put back the plain-address arm alone in aStopThatBindsTheMessage
// and this fails.
func TestAReviewSeesAStopKeptAsTheHashOfTheAddress(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (address_hash, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'kept', 'system', 'user')`,
		storekit.SuppressionHash("kept@returning.test")); err != nil {
		t.Fatalf("planting the kept stop: %v", err)
	}
	binding, err := aStopThatBindsTheMessage(e.ctx, e.store.db, ids.New[ids.ContactKind](), RefusedRecipient{
		Address: "kept@returning.test", Category: string(commsauthz.CategoryRequestedFollowup),
	})
	if err != nil {
		t.Fatalf("asking the review: %v", err)
	}
	if binding != commsauthz.ReasonSubjectRequest {
		t.Errorf("the review reads binding %q, want the kept stop", binding)
	}
}

// leadAdminCtx is e.ctx with the lead grants a lift of a lead's stop asks for,
// beside the contact grants it already holds.
func leadAdminCtx(e *channelConsentEnv) context.Context {
	return principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"lead":    {Create: true, Read: true, Update: true, Delete: true},
				"contact": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestALeadsStopIsListedAndLiftedThroughTheLead(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := leadAdminCtx(e)
	lead := ids.New[ids.LeadKind]()
	stop := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO lead (id, full_name, email, source, captured_by)
		VALUES ($1, 'Lena Lead', 'lena@lead.test', 'manual', 'human:x')`, lead); err != nil {
		t.Fatalf("planting the lead: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression
		    (id, lead_id, kind, source, captured_by, decided_by_level, recorded_through)
		VALUES ($1, $2, 'subject_request', 'recorded by a contact', $3, 'user', 'colleague')`,
		stop, lead, "human:"+e.user.String()); err != nil {
		t.Fatalf("planting the lead's stop: %v", err)
	}

	listed, err := e.store.ListLeadStops(ctx, lead)
	if err != nil {
		t.Fatalf("listing the lead's stops: %v", err)
	}
	if len(listed) != 1 || !listed[0].MayLift || !listed[0].RecordedByColleague {
		t.Fatalf("listed %+v, want the one stop, liftable and typed by a colleague", listed)
	}
	if err := e.store.Lift(ctx, LiftInput{ContactID: e.contact, SuppressionID: stop, Reason: "wrong record"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("lifting the lead's stop through a contact answered %v, want not found", err)
	}
	if err := e.store.Lift(ctx, LiftInput{LeadID: lead, SuppressionID: stop, Reason: "typed on the wrong lead"}); err != nil {
		t.Fatalf("lifting the lead's stop: %v", err)
	}
	if listed, err = e.store.ListLeadStops(ctx, lead); err != nil || len(listed) != 0 {
		t.Errorf("after the lift the lead lists %+v (%v), want none", listed, err)
	}
}

// carryAndDropOriginal carries the contact's one stop onto a new survivor and
// deletes the original, the way a later erasure of the merged-away record
// would. It answers the survivor.
func carryAndDropOriginal(t *testing.T, e *channelConsentEnv) ids.ContactID {
	t.Helper()
	survivor := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Survivor', 'test', 'human:x')`,
		survivor); err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.CarryStopsTx(e.ctx, tx,
			commsauthz.ContactStopSubject(e.contact), commsauthz.ContactStopSubject(survivor))
	}); err != nil {
		t.Fatalf("carrying the stop: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM communication_suppression WHERE contact_id = $1`, e.contact); err != nil {
		t.Fatalf("deleting the original: %v", err)
	}
	return survivor
}

// A copy whose original is gone keeps how the stop was recorded. The contact's
// own press stays theirs; a colleague's typed objection stays correctable.
//
// Mutation: drop recorded_through from the carry's INSERT and both fail.
func TestACopyKeepsHowItsStopWasRecordedAfterTheOriginalIsGone(t *testing.T) {
	t.Run("the contact's press", func(t *testing.T) {
		e := setupChannelConsent(t)
		if _, err := e.store.PublicStop(systemCtx(e.ws), e.contact, StopAllMarketing, "No more ads."); err != nil {
			t.Fatalf("the contact's own press: %v", err)
		}
		stop := onlyStop(t, e, e.ctx, carryAndDropOriginal(t, e))
		if stop.RecordedByColleague || stop.MayLift || stop.Correction {
			t.Errorf("the carried press reads %+v, want the contact's own act, not liftable", stop)
		}
	})
	t.Run("a colleague's typed objection", func(t *testing.T) {
		e := setupChannelConsent(t)
		if err := e.store.Suppress(e.ctx, SuppressInput{
			ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "typed by mistake",
		}); err != nil {
			t.Fatalf("recording the objection: %v", err)
		}
		stop := onlyStop(t, e, e.ctx, carryAndDropOriginal(t, e))
		if !stop.RecordedByColleague || !stop.Correction {
			t.Errorf("the carried objection reads %+v, want a colleague's, correctable", stop)
		}
		if stop.RecordedByName != nil {
			t.Errorf("the copy names %q, want no name: its captured_by is whoever merged", *stop.RecordedByName)
		}
	})
}

// A merge moves the retiring contact's addresses. Before the first one leaves,
// its subject-wide stops are bound to the addresses it held, whether the merge
// code or the database trigger does it.
//
// Mutation: drop the stop statement from BindRequestsToAddressesTx and the
// first case fails; drop the trigger and the second fails.
func TestAMergeBindsASubjectWideStopToItsOwnRecordsAddresses(t *testing.T) {
	for _, viaTrigger := range []bool{false, true} {
		t.Run(map[bool]string{false: "the merge code", true: "the trigger"}[viaTrigger], func(t *testing.T) {
			e := setupChannelConsent(t)
			survivor := ids.New[ids.ContactKind]()
			ctx := context.Background()
			for _, stmt := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Survivor', 'test', 'human:x')`, []any{survivor}},
				{`INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'tilda@first.test', 'manual', 'human:x')`, []any{e.contact}},
				{`INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'sven@second.test', 'manual', 'human:x')`, []any{survivor}},
				{`INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
				  VALUES ($1, 'subject_request', 'test', 'human:x', 'user')`, []any{e.contact}},
			} {
				if _, err := e.owner.Exec(ctx, stmt.sql, stmt.args...); err != nil {
					t.Fatalf("planting the pair: %v", err)
				}
			}
			if viaTrigger {
				if _, err := e.owner.Exec(ctx,
					`UPDATE contact_email SET contact_id = $2 WHERE contact_id = $1`, e.contact, survivor); err != nil {
					t.Fatalf("moving the address: %v", err)
				}
			} else if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				return e.store.BindRequestsToAddressesTx(e.ctx, tx, e.contact, survivor)
			}); err != nil {
				t.Fatalf("binding: %v", err)
			}
			var bound []string
			if err := e.owner.QueryRow(ctx, `
				SELECT recorded_for_addresses FROM communication_suppression WHERE contact_id = $1`,
				e.contact).Scan(&bound); err != nil {
				t.Fatalf("reading the binding: %v", err)
			}
			if len(bound) != 1 || bound[0] != "tilda@first.test" {
				t.Errorf("the stop is bound to %v, want only the retiring record's own address", bound)
			}
		})
	}
}
