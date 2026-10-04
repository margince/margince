// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealWatchPerms reads everything the watch card names: the deal, the
// contact, the message and the employment edge between contact and account.
var dealWatchPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"deal": {Read: true}, "contact": {Read: true}, "activity": {Read: true},
		"relationship": {Read: true}, "company": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// dealWatch is one deal on an account whose employee committed to something
// in a captured message.
type dealWatch struct {
	*integration.Env
	deal, contact, claim ids.UUID
}

func seedDealWatch(t *testing.T) dealWatch {
	t.Helper()
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company)
	subject, body := "Purchase order", "We will send the purchase order by Friday."
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the message: %v", err)
	}
	claim, err := e.Contacts.RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: claimKindTheirs,
		Body: "Send the purchase order", ActivityID: ids.UUID(message.Id),
		Quote: "We will send the purchase order by Friday.", Source: "extraction",
	})
	if err != nil {
		t.Fatalf("filing the commitment: %v", err)
	}
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Rollout", pipeline, stage, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company)
	return dealWatch{Env: e, deal: deal, contact: contact, claim: ids.UUID(claim.Id)}
}

// The deal page watches what the account's people committed to, with the
// words and the message to check it against.
func TestADealWatchesWhatItsCustomerCommittedTo(t *testing.T) {
	w := seedDealWatch(t)
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil {
		t.Fatalf("reading the deal's commitments: %v", err)
	}
	if !got.Complete || len(got.Data) != 1 {
		t.Fatalf("want one commitment and a complete list, got %d (complete=%v)", len(got.Data), got.Complete)
	}
	if row := got.Data[0]; ids.UUID(row.Id) != w.claim || row.ContactName != "Ines Huber" ||
		row.SourceQuote != "We will send the purchase order by Friday." {
		t.Errorf("the row reads %+v, want the claim, its contact and its words", row)
	}
}

// A commitment a rep dismissed is not watched any more.
func TestADismissedCommitmentLeavesTheDeal(t *testing.T) {
	w := seedDealWatch(t)
	if err := w.Contacts.SettleConversationClaim(w.Admin(), w.claim, "dismissed"); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil || len(got.Data) != 0 || !got.Complete {
		t.Errorf("a dismissed commitment reads as %d row(s), complete=%v, err=%v", len(got.Data), got.Complete, err)
	}
}

// A commitment whose contact the reader may not see is left out, and the list
// says it is incomplete rather than that the customer owes nothing.
func TestAHiddenCommitmentMakesTheListIncomplete(t *testing.T) {
	w := seedDealWatch(t)
	w.MakeCapturePrivate(t, "contact", w.contact, w.Rep3)
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil || len(got.Data) != 0 || got.Complete {
		t.Errorf("a hidden commitment reads as %d row(s), complete=%v, err=%v; want none and incomplete",
			len(got.Data), got.Complete, err)
	}
}

// A reader without the contact grant is refused rather than told nothing is owed.
func TestADealWatchNeedsTheContactGrant(t *testing.T) {
	w := seedDealWatch(t)
	perms := principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "company": {Read: true}, "activity": {Read: true},
		}}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, perms)
	if _, err := newDealCommitmentHandlers(w.Pool).read(reader, ids.From[ids.DealKind](w.deal)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader with no contact grant: err = %v, want permission denied", err)
	}
}

// A reader who may not see the deal's company has no account to read from,
// and the list says it is incomplete rather than that nothing is owed.
func TestAMaskedCompanyLeavesTheDealWatchIncomplete(t *testing.T) {
	w := seedDealWatch(t)
	perms := principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, perms)
	got, err := newDealCommitmentHandlers(w.Pool).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil || len(got.Data) != 0 || got.Complete {
		t.Errorf("a masked company reads as %d row(s), complete=%v, err=%v; want none and incomplete",
			len(got.Data), got.Complete, err)
	}
}
