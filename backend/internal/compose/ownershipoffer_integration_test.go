// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The owner of a contact or company offers it to a colleague, who accepts or
// declines it in their approvals inbox.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// handoverRole is handoverRepPerms as a stored role. An offer reads the stored
// grants of the colleague it names and of the owner it moves the record as.
const handoverRole = `{"objects":{` +
	`"contact":{"create":true,"read":true,"update":true},` +
	`"company":{"create":true,"read":true,"update":true},` +
	`"deal":{"create":true,"read":true,"update":true},` +
	`"activity":{"create":true,"read":true,"update":true},` +
	`"relationship":{"create":true,"read":true,"update":true},` +
	`"pipeline":{"read":true}},"row_scope":"own"}`

func grantHandoverRole(t *testing.T, e *integration.Env, members ...ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ('handover-rep', 'Handover rep', $1::jsonb)
		ON CONFLICT (key) DO NOTHING`, handoverRole)
	for _, member := range members {
		e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
			SELECT id, $1 FROM role WHERE key = 'handover-rep'`, member)
	}
}

func seedMember(t *testing.T, e *integration.Env, name, seat, status string) ids.UUID {
	t.Helper()
	member := ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, seat_type, status) VALUES ($1, $2, $3, $4, $5)`,
		member, name+"@offer.test", name, seat, status)
	return member
}

func receiver(e *integration.Env) context.Context { return asker(e) }

func offerHandover(t *testing.T, e *integration.Env, recordType string, id, to ids.UUID) (ownershipRequestView, bool) {
	t.Helper()
	out, created, err := newOwnershipTransfers(e.Pool).Offer(giver(e), recordType, id, to, "You run this account now")
	if err != nil {
		t.Fatalf("offering the %s: %v", recordType, err)
	}
	return out, created
}

// The colleague accepts. The company, its open deal and the owner's open task
// on it move to them, each with its own audit row. The colleague is outside the
// owner's own scope, which a direct reassignment would refuse.
func TestAnAcceptedOfferMovesTheRecordAndTheOwnersOpenWorkToTheColleague(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep3)
	company := e.SeedCompany(t, "Offered AG", &e.Rep1)
	stages := newHandoverPipeline(t, e)
	openDeal := dealOn(t, e, stages, "Offered renewal", e.Rep1, company)
	openTask := taskOn(t, e, "Call Offered back", e.Rep1, linkTo("company", company))
	if err := database.WithWorkspaceTx(giver(e), e.Pool, func(tx pgx.Tx) error {
		return auth.EnsureAssignee(giver(e), tx, e.Rep3)
	}); err == nil {
		t.Fatalf("the owner may assign to the colleague directly, so this case proves nothing about the scope bound")
	}

	offer, created := offerHandover(t, e, "company", company, e.Rep3)
	if !created || offer.Transfer.DeciderID != e.Rep3 || offer.Transfer.RequestedBy != e.Rep1 {
		t.Fatalf("offer = %+v (created %v), want a new offer put to the colleague", offer, created)
	}
	summary := e.WsScalar(t, `SELECT summary FROM approval WHERE id = $1`, offer.ApprovalID)
	if !strings.Contains(summary, "wants to hand you this company. Open deals that move with it: 1. Open tasks: 1.") {
		t.Errorf("the card reads %q, want the offer with one open deal and one open task", summary)
	}
	if n := transfersInInbox(receiver(e), t, e); n != 1 {
		t.Fatalf("the colleague sees %d offers, want 1", n)
	}
	if err := decideHandover(receiver(e), t, e, offer.ApprovalID, true); err != nil {
		t.Fatalf("the colleague accepting: %v", err)
	}

	if got := ownerOf(t, e, "company", company); got != e.Rep3 {
		t.Errorf("the company is owned by %s, want the colleague", got)
	}
	if got := ownerOf(t, e, "deal", openDeal); got != e.Rep3 {
		t.Errorf("the open deal is owned by %s, want it moved with the company", got)
	}
	if got := assigneeOf(t, e, openTask); got != e.Rep3 {
		t.Errorf("the open task is held by %s, want it moved with the company", got)
	}
	for table, id := range map[string]ids.UUID{"company": company, "deal": openDeal} {
		if n := ownerChangeAudits(t, e, table, id, e.Rep1, e.Rep3); n != 1 {
			t.Errorf("%s owner-change audit rows = %d, want 1", table, n)
		}
	}
	if decidedBy := e.WsScalar(t, `SELECT decided_by::text FROM approval WHERE id = $1`, offer.ApprovalID); decidedBy != e.Rep3.String() {
		t.Errorf("the offer was decided by %s, want the colleague who accepted", decidedBy)
	}
	subject := e.WsScalar(t, `SELECT subject FROM notice WHERE recipient_user_id = $1 AND kind = $2`, e.Rep1, transferAcceptedKind)
	if !strings.Contains(subject, "took over Offered AG") {
		t.Errorf("the owner is told %q, want that the colleague took the company over", subject)
	}
}

// Declined and run out: the record stays with its owner, who is told each
// time.
func TestADeclinedOrExpiredOfferLeavesTheRecordAndTellsTheOwner(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep3)
	contact := e.SeedContact(t, "Kept Contact", &e.Rep1)

	first, _ := offerHandover(t, e, "contact", contact, e.Rep3)
	if err := decideHandover(receiver(e), t, e, first.ApprovalID, false); err != nil {
		t.Fatalf("the colleague declining: %v", err)
	}
	if got := ownerOf(t, e, "contact", contact); got != e.Rep1 {
		t.Fatalf("a declined offer moved the contact to %s", got)
	}
	if n := noticesOf(t, e, e.Rep1, transferDeclined.kind); n != 1 {
		t.Errorf("decline notices to the owner = %d, want 1", n)
	}

	second, created := offerHandover(t, e, "contact", contact, e.Rep3)
	if !created || second.ApprovalID == first.ApprovalID {
		t.Fatalf("offering again after a decline answered %+v (created %v), want a new offer", second, created)
	}
	if window := e.WsScalar(t, `SELECT (expires_at - created_at)::text FROM approval WHERE id = $1`, second.ApprovalID); window != "7 days" {
		t.Errorf("the offer stays open for %s, want 7 days", window)
	}
	e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, second.ApprovalID)
	if _, err := expiringApprovalsService(e.Pool).ExpireDue(expiryCtx(e)); err != nil {
		t.Fatalf("the expiry sweep: %v", err)
	}
	if got := ownerOf(t, e, "contact", contact); got != e.Rep1 {
		t.Fatalf("an expired offer moved the contact to %s", got)
	}
	if n := noticesOf(t, e, e.Rep1, transferExpired.kind); n != 1 {
		t.Errorf("expiry notices to the owner = %d, want 1", n)
	}
}

// Only the owner offers, and only to an active colleague on a seat that can
// write and a role that may change the record. Every refusal files nothing.
func TestOnlyTheOwnerOffersARecordToAColleagueWhoCanHoldIt(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep2, e.Rep3)
	company := e.SeedCompany(t, "Guarded AG", &e.Rep1)
	transfers := newOwnershipTransfers(e.Pool)
	manager := handoverRepPerms()
	manager.RowScope = principal.RowScopeAll
	readSeat := seedMember(t, e, "reader", "read", "active")
	suspended := seedMember(t, e, "away", "full", "suspended")
	noRole := seedMember(t, e, "norole", "full", "active")
	grantHandoverRole(t, e, readSeat, suspended)

	for name, c := range map[string]struct {
		offerer context.Context
		to      ids.UUID
		// want is nil for a colleague refused on to_id.
		want error
	}{
		"a manager who is not the owner":   {seatedAs(e, e.Rep2, manager), e.Rep3, apperrors.ErrPermissionDenied},
		"the owner's agent":                {e.AgentFor(t, e.Rep1, nil, handoverRepPerms()), e.Rep3, apperrors.ErrPermissionDenied},
		"to the owner":                     {giver(e), e.Rep1, nil},
		"to a read seat":                   {giver(e), readSeat, nil},
		"to a suspended seat":              {giver(e), suspended, nil},
		"to a colleague who cannot change": {giver(e), noRole, nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := transfers.Offer(c.offerer, "company", company, c.to, "")
			if c.want == nil {
				if fault, ok := errors.AsType[*offerReceiverError](err); !ok {
					t.Fatalf("offering → %v, want 422 on to_id", err)
				} else if field, code, _ := fault.FieldFault(); field != "to_id" || code != "owner_not_assignable" {
					t.Errorf("the refusal names %s/%s, want to_id/owner_not_assignable", field, code)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("offering → %v, want %v", err, c.want)
			}
		})
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1`, ownershipTransferKind); n != 0 {
		t.Errorf("%d offers filed by refused offers", n)
	}
}

// One open handover per record. A colleague's open request blocks an offer.
// An offer blocks a second offer and a request. A second press for the same
// colleague joins the open offer.
func TestOneHandoverOfARecordIsOpenAtATime(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep2, e.Rep3)
	company := e.SeedCompany(t, "Busy AG", &e.Rep1)
	transfers := newOwnershipTransfers(e.Pool)

	requestHandover(asker(e), t, e, "company", company)
	_, _, err := transfers.Offer(giver(e), "company", company, e.Rep2, "")
	if code := transferRefusalCode(err); !errors.Is(err, apperrors.ErrConflict) || code != "transfer_pending" {
		t.Fatalf("offering beside an open request → %v (%q), want 409 transfer_pending", err, code)
	}
	if err := transfers.Withdraw(asker(e), "company", company, transferAsked); err != nil {
		t.Fatalf("withdrawing the request: %v", err)
	}

	offer, _ := offerHandover(t, e, "company", company, e.Rep2)
	again, created := offerHandover(t, e, "company", company, e.Rep2)
	if created || again.ApprovalID != offer.ApprovalID {
		t.Errorf("a second press for the same colleague answered %s (created %v), want the open offer %s", again.ApprovalID, created, offer.ApprovalID)
	}
	_, _, err = transfers.Offer(giver(e), "company", company, e.Rep3, "")
	if code := transferRefusalCode(err); code != "transfer_pending" || !strings.Contains(err.Error(), "already offered to") {
		t.Errorf("offering to a second colleague → %v (%q), want 409 naming the open offer", err, code)
	}
	_, _, err = transfers.Request(asker(e), "company", company, "")
	if code := transferRefusalCode(err); code != "transfer_pending" {
		t.Errorf("asking beside an open offer → %v (%q), want 409 transfer_pending", err, code)
	}
	if _, err := transfers.Own(giver(e), "company", company, transferAsked); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the owner reading their offer as a request → %v, want 404", err)
	}
	if err := transfers.Withdraw(giver(e), "company", company, transferOffered); err != nil {
		t.Fatalf("the owner withdrawing the offer: %v", err)
	}
	if n := transfersInInbox(seatedAs(e, e.Rep2, handoverRepPerms()), t, e); n != 0 {
		t.Errorf("a withdrawn offer still waits in the colleague's inbox (%d)", n)
	}
}

// The card is the colleague's alone. Neither the owner, another colleague nor
// an all-scope manager sees or decides it, and the colleague's agent may
// decline it but not accept it.
func TestOnlyTheColleagueOfferedTheRecordDecidesIt(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep3)
	company := e.SeedCompany(t, "Addressed AG", &e.Rep1)
	offer, _ := offerHandover(t, e, "company", company, e.Rep3)

	for name, reader := range map[string]context.Context{
		"owner":             giver(e),
		"another colleague": seatedAs(e, e.Rep2, handoverRepPerms()),
		"all-scope manager": seatedAs(e, e.AdminUser, integration.AdminPerms),
	} {
		if n := transfersInInbox(reader, t, e); n != 0 {
			t.Errorf("the %s sees %d offers put to the colleague", name, n)
		}
		if err := decideHandover(reader, t, e, offer.ApprovalID, true); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("the %s accepting → %v, want the card absent", name, err)
		}
	}
	agent := e.AgentFor(t, e.Rep3, nil, handoverRepPerms())
	if err := decideHandover(agent, t, e, offer.ApprovalID, true); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the colleague's agent accepting → %v, want 403", err)
	}
	if err := decideHandover(agent, t, e, offer.ApprovalID, false); err != nil {
		t.Errorf("the colleague's agent declining → %v, want it allowed", err)
	}
	if got := ownerOf(t, e, "company", company); got != e.Rep1 {
		t.Errorf("the company moved to %s", got)
	}
}

// A party who can no longer give or take the record leaves the offer pending
// when its colleague accepts: the refusal comes before the decision. A giver
// who left is refused by the precheck and the card can still be declined. A
// receiver whose role lost the grant likewise. A receiver moved to a read seat
// no longer sees the card, and the owner withdraws it.
func TestAnOfferEitherPartyCanNoLongerHonourStaysPending(t *testing.T) {
	for name, c := range map[string]struct {
		lapse    func(t *testing.T, e *integration.Env)
		wantCode string
	}{
		"suspended giver": {func(t *testing.T, e *integration.Env) {
			e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep1)
		}, "owner_unavailable"},
		"departed giver": {func(t *testing.T, e *integration.Env) { deactivate(t, e, e.Rep1) }, "owner_unavailable"},
		"receiver without the grant": {func(t *testing.T, e *integration.Env) {
			e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.Rep3)
		}, "receiver_unavailable"},
		"receiver on a read seat": {func(t *testing.T, e *integration.Env) {
			e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.Rep3)
		}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			grantHandoverRole(t, e, e.Rep1, e.Rep3)
			company := e.SeedCompany(t, "Lapsed AG", &e.Rep1)
			offer, _ := offerHandover(t, e, "company", company, e.Rep3)
			c.lapse(t, e)

			err := decideHandover(receiver(e), t, e, offer.ApprovalID, true)
			if c.wantCode == "" {
				if !errors.Is(err, apperrors.ErrNotFound) {
					t.Fatalf("accepting → %v, want the card gone", err)
				}
			} else if code := transferRefusalCode(err); code != c.wantCode {
				t.Fatalf("accepting → %v (%q), want %s", err, code, c.wantCode)
			}
			if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, offer.ApprovalID); status != "pending" {
				t.Fatalf("a refused accept left the offer %s, want it pending", status)
			}
			if got := ownerOf(t, e, "company", company); got != e.Rep1 {
				t.Errorf("a refused accept moved the company to %s", got)
			}
			if c.wantCode == "" {
				if err := newOwnershipTransfers(e.Pool).Withdraw(giver(e), "company", company, transferOffered); err != nil {
					t.Fatalf("the owner withdrawing the offer: %v", err)
				}
				return
			}
			if err := decideHandover(receiver(e), t, e, offer.ApprovalID, false); err != nil {
				t.Fatalf("declining after the refused accept: %v", err)
			}
			if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, offer.ApprovalID); status != "rejected" {
				t.Errorf("the declined offer is %s, want rejected", status)
			}
		})
	}
}

// The picker names the colleagues an offer would accept and no one else, and
// only the owner reads it.
func TestTheOfferPickerNamesOnlyColleaguesWhoCanTakeTheRecord(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep3)
	company := e.SeedCompany(t, "Picked AG", &e.Rep1)
	readSeat := seedMember(t, e, "reader", "read", "active")
	suspended := seedMember(t, e, "away", "full", "suspended")
	noRole := seedMember(t, e, "norole", "full", "active")
	grantHandoverRole(t, e, readSeat, suspended)
	transfers := newOwnershipTransfers(e.Pool)

	candidates, err := transfers.OfferCandidates(giver(e), "company", company)
	if err != nil {
		t.Fatalf("reading the candidates: %v", err)
	}
	named := map[ids.UUID]bool{}
	for _, candidate := range candidates {
		named[candidate.ID] = true
	}
	if !named[e.Rep3] {
		t.Errorf("the picker leaves out a colleague who can take the record")
	}
	for who, id := range map[string]ids.UUID{
		"the owner": e.Rep1, "a read seat": readSeat, "a suspended seat": suspended, "a colleague without the grant": noRole,
	} {
		if named[id] {
			t.Errorf("the picker offers %s", who)
		}
	}
	manager := handoverRepPerms()
	manager.RowScope = principal.RowScopeAll
	for name, reader := range map[string]context.Context{
		"a manager who is not the owner": seatedAs(e, e.Rep2, manager),
		"a colleague":                    receiver(e),
	} {
		if _, err := transfers.OfferCandidates(reader, "company", company); !errors.Is(err, apperrors.ErrPermissionDenied) && !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s reading the candidates → %v, want 403 or 404", name, err)
		}
	}
}

// The route answers 201 for a new offer and 200 for the one already open.
func TestTheOfferRouteTellsANewOfferFromAnOpenOne(t *testing.T) {
	e := integration.Setup(t)
	grantHandoverRole(t, e, e.Rep1, e.Rep3)
	company := e.SeedCompany(t, "Route Offer AG", &e.Rep1)
	h := ownershipRequestHandlers{transfers: newOwnershipTransfers(e.Pool)}
	body, err := json.Marshal(crmcontracts.OwnershipOfferInput{ToId: openapi_types.UUID(e.Rep3)})
	if err != nil {
		t.Fatalf("encoding the offer: %v", err)
	}
	post := func() (int, crmcontracts.OwnershipOffer) {
		req := httptest.NewRequest(http.MethodPost, "/v1/records/company/"+company.String()+"/ownership-offer",
			strings.NewReader(string(body))).WithContext(giver(e))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.OfferOwnershipTransfer(rec, req, "company", crmcontracts.Id(company), crmcontracts.OfferOwnershipTransferParams{})
		var out crmcontracts.OwnershipOffer
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
		t.Fatalf("two presses answered %d then %d (%s, %s), want 201 then 200 on one offer",
			first, second, made.ApprovalId, joined.ApprovalId)
	}
	if ids.UUID(made.ToId) != e.Rep3 {
		t.Errorf("the offer names %s, want the colleague", made.ToId)
	}
}
