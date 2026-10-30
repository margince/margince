// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who may ask for a record, who sees the request, and who may answer it.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func transferRefusalCode(err error) string {
	if refusal, ok := errors.AsType[*contacts.TransferRefusal](err); ok {
		return refusal.Code
	}
	return ""
}

// Every record that is not a colleague's to give answers with the refusal that
// says why, and files nothing.
func TestOnlyAColleagueWhoCannotChangeARecordMayAskForIt(t *testing.T) {
	e := integration.Setup(t)
	transfers := newOwnershipTransfers(e.Pool)
	owned := e.SeedCompany(t, "Rep1 GmbH", &e.Rep1)
	unowned := e.SeedCompany(t, "Nobody GmbH", nil)
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](unowned),
		contacts.UpdateCompanyInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatalf("clearing the owner: %v", err)
	}
	private := e.SeedContact(t, "Quiet Contact", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	noUpdate := handoverRepPerms()
	noUpdate.Objects["company"] = principal.ObjectGrant{Read: true}
	readSeat := asker(e)
	if p, ok := principal.Actor(readSeat); ok {
		p.SeatType = principal.SeatRead
		readSeat = principal.WithActor(readSeat, p)
	}

	for name, c := range map[string]struct {
		ask      func() error
		want     error
		wantCode string
	}{
		"own record": {ask: func() error {
			_, _, err := transfers.Request(giver(e), "company", owned, "")
			return err
		}, want: apperrors.ErrConflict, wantCode: "already_yours_to_change"},
		"unowned record": {ask: func() error {
			_, _, err := transfers.Request(asker(e), "company", unowned, "")
			return err
		}, want: apperrors.ErrConflict, wantCode: "unowned"},
		"no update grant": {ask: func() error {
			_, _, err := transfers.Request(seatedAs(e, e.Rep3, noUpdate), "company", owned, "")
			return err
		}, want: apperrors.ErrPermissionDenied},
		"read seat": {ask: func() error {
			_, _, err := transfers.Request(readSeat, "company", owned, "")
			return err
		}, want: apperrors.ErrPermissionDenied},
		"agent": {ask: func() error {
			_, _, err := transfers.Request(e.AgentFor(t, e.Rep3, nil, handoverRepPerms()), "company", owned, "")
			return err
		}, want: apperrors.ErrPermissionDenied},
		"owner-private contact": {ask: func() error {
			_, _, err := transfers.Request(asker(e), "contact", private, "")
			return err
		}, want: apperrors.ErrNotFound},
		"unknown record": {ask: func() error {
			_, _, err := transfers.Request(asker(e), "company", ids.NewV7(), "")
			return err
		}, want: apperrors.ErrNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			err := c.ask()
			if !errors.Is(err, c.want) {
				t.Fatalf("asking → %v, want %v", err, c.want)
			}
			if code := transferRefusalCode(err); code != c.wantCode {
				t.Errorf("refusal code = %q, want %q", code, c.wantCode)
			}
		})
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1`, ownershipTransferKind); n != 0 {
		t.Errorf("%d requests filed by refused asks", n)
	}

	deactivate(t, e, e.Rep1)
	_, _, err := transfers.Request(asker(e), "company", owned, "")
	if code := transferRefusalCode(err); !errors.Is(err, apperrors.ErrConflict) || code != "owner_unavailable" {
		t.Errorf("asking a departed owner → %v (%q), want 409 owner_unavailable", err, code)
	}
}

// The card is the owner's alone. An administrator does not see it, and the
// owner's agent may decline it but not accept it. The colleague who asked
// reads and withdraws only their own request.
func TestOnlyTheOwnerAnswersAndOnlyTheAskerWithdraws(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Private Card AG", &e.Rep1)
	request, _ := requestHandover(t, e, asker(e), "company", company)
	transfers := newOwnershipTransfers(e.Pool)

	if n := transfersInInbox(t, e, giver(e)); n != 1 {
		t.Errorf("the owner sees %d requests, want 1", n)
	}
	for name, reader := range map[string]context.Context{
		"administrator": seatedAs(e, e.AdminUser, integration.AdminPerms), "asker": asker(e),
	} {
		if n := transfersInInbox(t, e, reader); n != 0 {
			t.Errorf("the %s sees %d requests put to the owner", name, n)
		}
	}
	agent := e.AgentFor(t, e.Rep1, nil, handoverRepPerms())
	if err := decideHandover(t, e, agent, request.ApprovalID, true); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the owner's agent accepting → %v, want 403", err)
	}
	if _, err := transfers.Own(seatedAs(e, e.Rep2, handoverRepPerms()), "company", company); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another colleague reading the request → %v, want 404", err)
	}
	if err := transfers.Withdraw(seatedAs(e, e.Rep2, handoverRepPerms()), "company", company); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another colleague withdrawing the request → %v, want 404", err)
	}
	own, err := transfers.Own(asker(e), "company", company)
	if err != nil || own.ApprovalID != request.ApprovalID {
		t.Fatalf("the asker reading their request → %+v, %v", own, err)
	}
	if err := transfers.Withdraw(asker(e), "company", company); err != nil {
		t.Fatalf("the asker withdrawing: %v", err)
	}
	if n := transfersInInbox(t, e, giver(e)); n != 0 {
		t.Errorf("a withdrawn request still waits in the owner's inbox (%d)", n)
	}

	again, _ := requestHandover(t, e, asker(e), "company", company)
	if err := decideHandover(t, e, agent, again.ApprovalID, false); err != nil {
		t.Errorf("the owner's agent declining → %v, want it allowed", err)
	}
}

// The route answers 201 for a new request and 200 for the one already open.
func TestTheRequestRouteTellsANewRequestFromAnOpenOne(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Route AG", &e.Rep1)
	h := ownershipRequestHandlers{transfers: newOwnershipTransfers(e.Pool)}
	post := func() (int, crmcontracts.OwnershipRequest) {
		req := httptest.NewRequest(http.MethodPost, "/v1/records/company/"+company.String()+"/ownership-request", nil).
			WithContext(asker(e))
		rec := httptest.NewRecorder()
		h.RequestOwnershipTransfer(rec, req, "company", crmcontracts.Id(company), crmcontracts.RequestOwnershipTransferParams{})
		var out crmcontracts.OwnershipRequest
		if rec.Code < 300 {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("reading %q: %v", rec.Body.String(), err)
			}
		}
		return rec.Code, out
	}
	first, made := post()
	second, joined := post()
	if first != http.StatusCreated || second != http.StatusOK || made.ApprovalId != joined.ApprovalId {
		t.Fatalf("two presses answered %d then %d (%s, %s), want 201 then 200 on one request",
			first, second, made.ApprovalId, joined.ApprovalId)
	}
	if ids.UUID(made.OwnerId) != e.Rep1 {
		t.Errorf("the request names %s, want the owner", made.OwnerId)
	}
}
