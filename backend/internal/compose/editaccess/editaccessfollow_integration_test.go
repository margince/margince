// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package editaccess_test

// What happens to a request while it waits. The record moves, or the window
// runs out, or someone else tries to answer it. The colleague who asked may
// lose sight of the record, or the lead may be erased.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/editaccess"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func ask(t *testing.T, e *integration.Env, kind string, record ids.UUID) editaccess.Request {
	t.Helper()
	request, _, err := engine(e).Ask(seated(e, e.Rep3, principal.SeatFull), kind, record)
	if err != nil {
		t.Fatalf("asking about the %s: %v", kind, err)
	}
	return request
}

func statusOf(t *testing.T, e *integration.Env, approval ids.ApprovalID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, approval)
}

func noticesTo(t *testing.T, e *integration.Env, member ids.UUID, kind string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = $2`, member, kind)
}

// A deal moved through its writer closes the request put to its former owner,
// and the colleague who asked is told.
func TestADealThatChangedHandsClosesItsEditAccessRequest(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep2, e.Rep3)
	deal := seedRecord(t, e, "deal", e.Rep1)
	request := ask(t, e, "deal", deal)

	newOwner := ids.From[ids.UserKind](e.Rep2)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &newOwner}); err != nil {
		t.Fatalf("moving the deal: %v", err)
	}
	if status := statusOf(t, e, request.ApprovalID); status == "pending" {
		t.Errorf("the request put to the former owner is still pending")
	}
	if n := noticesTo(t, e, e.Rep3, "edit_access_withdrawn"); n != 1 {
		t.Errorf("notices that the request closed = %d, want 1", n)
	}
}

// A lead tells nobody when it moves. So the next ask closes the request put to
// its former owner, and puts a new one to its owner now.
func TestAskingAboutALeadThatChangedHandsAsksItsNewOwner(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep2, e.Rep3)
	lead := seedRecord(t, e, "lead", e.Rep1)
	first := ask(t, e, "lead", lead)

	newOwner := ids.From[ids.UserKind](e.Rep2)
	if _, err := e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](lead), contacts.UpdateLeadInput{OwnerID: &newOwner}); err != nil {
		t.Fatalf("moving the lead: %v", err)
	}
	second := ask(t, e, "lead", lead)
	if second.ApprovalID == first.ApprovalID || second.Ask.OwnerID != e.Rep2 {
		t.Errorf("the second ask = %+v, want a new request put to the new owner", second)
	}
	if status := statusOf(t, e, first.ApprovalID); status == "pending" {
		t.Errorf("the request put to the former owner is still pending")
	}
}

// Only the owner the request is put to gives the access. An administrator who
// may change the lead does not.
func TestOnlyTheOwnerGivesEditAccess(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	lead := seedRecord(t, e, "lead", e.Rep1)
	request := ask(t, e, "lead", lead)

	if _, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(e.Admin(), request.ApprovalID, true, nil); err == nil {
		t.Error("an administrator approved a request put to the lead's owner")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_id = $1`, lead); n != 0 {
		t.Errorf("shares after the administrator's try = %d, want none", n)
	}
}

// A request nobody answered within its window tells the colleague who asked.
func TestAnExpiredEditAccessRequestTellsTheColleagueWhoAsked(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	project := seedRecord(t, e, "project", e.Rep1)
	request := ask(t, e, "project", project)
	e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, request.ApprovalID)

	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), approvals.ExpiryActor)
	if _, err := editaccess.RegisterExpiry(compose.NewApprovalsService(e.DB())).ExpireDue(sweep); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if n := noticesTo(t, e, e.Rep3, "edit_access_expired"); n != 1 {
		t.Errorf("expiry notices to the colleague who asked = %d, want 1", n)
	}
}

// A record the asker lost sight of answers as if it did not exist. That holds
// for a new ask, and for the request made while they could still see it.
func TestARecordOutsideTheAskersSightAnswersNotFound(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	contact := e.SeedContact(t, "Ines Brandt", &e.Rep1)
	ask(t, e, "contact", contact)
	hide(t, e, contact)

	asker := seated(e, e.Rep3, principal.SeatFull)
	if _, err := engine(e).Own(asker, "contact", contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a request on a contact the asker cannot see: %v, want not found", err)
	}
	if _, _, err := engine(e).Ask(asker, "contact", contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("asking about a contact the asker cannot see: %v, want not found", err)
	}
}

// hide makes a contact its owner's alone, through the contact's writer.
func hide(t *testing.T, e *integration.Env, contact ids.UUID) {
	t.Helper()
	private := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
}

// The line telling the colleague how their request ended names the record only
// while they may still see it.
func TestTheOutcomeLineNamesOnlyARecordTheColleagueStillSees(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	contact := e.SeedContact(t, "Ines Brandt", &e.Rep1)
	request := ask(t, e, "contact", contact)
	hide(t, e, contact)
	decide(t, e, request.ApprovalID, false)

	subject := e.WsScalar(t, `SELECT subject FROM notice WHERE recipient_user_id = $1 AND kind = 'edit_access_declined'`, e.Rep3)
	if strings.Contains(subject, "Ines Brandt") || !strings.Contains(subject, "a record") {
		t.Errorf("the decline line reads %q, want it to name no contact the colleague lost sight of", subject)
	}
}

// Erasing a website lead nobody could tell closes the requests on it, and nobody
// is sent a line about a lead that no longer exists.
func TestErasingAnUntoldLeadClosesItsEditAccessRequest(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	grantEditorRole(t, e, e.Rep1, e.Rep3)
	lead := seedRecord(t, e, "lead", e.Rep1)
	request := ask(t, e, "lead", lead)
	e.WsExec(t, `INSERT INTO lead_privacy_notice (lead_id, template_key, template_version, state, failure, failed_at)
		VALUES ($1, 'site_lead', 1, 'failed', 'refused', now())`, lead)

	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return privacy.EraseUntoldLeadTx(e.Admin(), tx, privacy.UntoldLead{LeadID: ids.From[ids.LeadKind](lead)})
	}); err != nil {
		t.Fatalf("erasing the untold lead: %v", err)
	}
	if status := statusOf(t, e, request.ApprovalID); status == "pending" {
		t.Errorf("the request on the erased lead is still pending")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind LIKE 'edit_access_%'`, e.Rep3); n != 0 {
		t.Errorf("lines to the colleague who asked = %d, want none", n)
	}
}
