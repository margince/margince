// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Correcting a stop recorded by mistake, and the list that offers it.
//
// A marketing objection is stamped at the subject's level whoever types it. An
// admin may take back one a colleague typed, with a reason; a rep may not, and
// nobody takes back one the person pressed themselves.

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// repOwnedContact seeds a contact the bounded rep owns, so the rep's own
// write gates pass and only the lift rule is under test.
func repOwnedContact(t *testing.T, e *channelConsentEnv) ids.ContactID {
	t.Helper()
	own := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, 'Rita Customer', 'test', 'human:x', 'workspace', $2)`,
		own, e.user); err != nil {
		t.Fatal(err)
	}
	return own
}

// systemCtx is the principal the public preference edge binds: the product
// acting on the person's own press.
func systemCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
}

func onlyStop(t *testing.T, e *channelConsentEnv, ctx context.Context, contact ids.ContactID) crmcontracts.ContactStop {
	t.Helper()
	stops, err := e.store.ListStops(ctx, contact)
	if err != nil {
		t.Fatalf("listing the stops: %v", err)
	}
	if len(stops) != 1 {
		t.Fatalf("listed %d stops, want 1: %+v", len(stops), stops)
	}
	return stops[0]
}

func TestAnAdminCorrectsAnObjectionAColleagueTyped(t *testing.T) {
	e := setupChannelConsent(t)
	contact := repOwnedContact(t, e)
	rep := boundedRepCtx(e.ws, e.user)
	if err := e.store.Suppress(rep, SuppressInput{
		ContactID: contact, Kind: commsauthz.ReasonObjection, Reason: "said no newsletters",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}

	asRep := onlyStop(t, e, rep, contact)
	if asRep.MayLift || asRep.LiftBlockedBy == nil ||
		*asRep.LiftBlockedBy != crmcontracts.ContactStopLiftBlockedBySubjectDecision {
		t.Errorf("the rep is offered %+v, want no lift and subject_decision", asRep)
	}
	asAdmin := onlyStop(t, e, e.ctx, contact)
	if !asAdmin.MayLift || !asAdmin.Correction || asAdmin.LiftBlockedBy != nil {
		t.Errorf("the admin is offered %+v, want a correction", asAdmin)
	}
	if asAdmin.Authority != crmcontracts.ContactStopAuthoritySubject || !asAdmin.RecordedByColleague ||
		asAdmin.RecordedByName == nil || *asAdmin.RecordedByName != "Rep" ||
		asAdmin.Reason == nil || *asAdmin.Reason != "said no newsletters" {
		t.Errorf("the stop reads %+v, want subject authority typed by Rep with its reason", asAdmin)
	}

	err := e.store.Lift(rep, LiftInput{ContactID: contact, SuppressionID: ids.UUID(asRep.Id), Reason: "mistake"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep correcting the objection answered %v, want ErrPermissionDenied", err)
	}
	var invalid *ValidationError
	err = e.store.Lift(e.ctx, LiftInput{ContactID: contact, SuppressionID: ids.UUID(asAdmin.Id)})
	if !errors.As(err, &invalid) {
		t.Fatalf("a correction with no reason answered %v, want a validation error", err)
	}
	if err := e.store.Lift(e.ctx, LiftInput{
		ContactID: contact, SuppressionID: ids.UUID(asAdmin.Id), Reason: "typed on the wrong contact",
	}); err != nil {
		t.Fatalf("the admin's correction: %v", err)
	}
	if stillLive(t, e, ids.UUID(asAdmin.Id)) {
		t.Error("the corrected objection is still live")
	}

	var payload string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT after::text FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND after ? 'lifted_suppression'`,
		contact).Scan(&payload); err != nil {
		t.Fatalf("the correction left no audit entry: %v", err)
	}
	for _, want := range []string{liftBasisCorrection, "typed on the wrong contact"} {
		if !strings.Contains(payload, want) {
			t.Errorf("the audit entry omits %q: %s", want, payload)
		}
	}
}

func TestNobodyCorrectsAnObjectionThePersonPressed(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.store.PublicStop(systemCtx(e.ws), e.contact, StopAllMarketing, "No more ads."); err != nil {
		t.Fatalf("the person's own press: %v", err)
	}
	stop := onlyStop(t, e, e.ctx, e.contact)
	if stop.MayLift || stop.RecordedByColleague {
		t.Errorf("the admin is offered %+v, want no lift on the person's own press", stop)
	}
	err := e.store.Lift(e.ctx, LiftInput{ContactID: e.contact, SuppressionID: ids.UUID(stop.Id), Reason: "resume"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin lifting the person's press answered %v, want ErrPermissionDenied", err)
	}
}

// A merge copies the person's press onto the survivor and stamps the merger.
// The copy is still the person's.
func TestACarriedPressStaysThePersons(t *testing.T) {
	e := setupChannelConsent(t)
	origin, carried := ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression
		    (id, contact_id, kind, source, captured_by, decided_by_level, carried_from)
		VALUES ($1, $3, 'marketing_objection', 'test', 'system', 'subject', NULL),
		       ($2, $3, 'marketing_objection', 'test', 'human:merger', 'subject', $1)`,
		origin, carried, e.contact); err != nil {
		t.Fatalf("planting the carried press: %v", err)
	}
	err := e.store.Lift(e.ctx, LiftInput{ContactID: e.contact, SuppressionID: carried, Reason: "resume"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin lifting a carried press answered %v, want ErrPermissionDenied", err)
	}
}

// No level lifts a restriction here: lifting one owes the person notice first.
func TestNoSeatLiftsARestriction(t *testing.T) {
	e := setupChannelConsent(t)
	row := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression
		    (id, contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, 'processing_restriction', 'test', 'human:x', 'user')`,
		row, e.contact); err != nil {
		t.Fatal(err)
	}
	stop := onlyStop(t, e, e.ctx, e.contact)
	if stop.MayLift || stop.LiftBlockedBy == nil ||
		*stop.LiftBlockedBy != crmcontracts.ContactStopLiftBlockedByRestriction {
		t.Errorf("the admin is offered %+v, want the restriction refusal", stop)
	}
	err := e.store.Lift(e.ctx, LiftInput{ContactID: e.contact, SuppressionID: row, Reason: "over"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin lifting a restriction answered %v, want ErrPermissionDenied", err)
	}
}
