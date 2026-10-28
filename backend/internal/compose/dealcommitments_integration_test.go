// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	pipeline             ids.PipelineID
	stage                ids.StageID
}

func seedDealWatch(t *testing.T) dealWatch {
	t.Helper()
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Rollout", pipeline, stage, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company)
	w := dealWatch{Env: e, deal: deal, contact: contact, pipeline: pipeline, stage: stage}
	w.claim = w.promiseOn(t, deal, "Purchase order", "We will send the purchase order by Friday.")
	return w
}

// promiseOn files a message on deal and the customer's commitment read out of
// it.
func (w dealWatch) promiseOn(t *testing.T, deal ids.UUID, subject, body string) ids.UUID {
	t.Helper()
	claim, _ := w.promiseWithSource(t, deal, subject, body)
	return claim
}

// promiseWithSource is promiseOn answering the message the promise cites too.
func (w dealWatch) promiseWithSource(t *testing.T, deal ids.UUID, subject, body string) (claimID, source ids.UUID) {
	t.Helper()
	message, _, err := w.Activities.LogActivity(w.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact}, {EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatalf("logging the message: %v", err)
	}
	claim, err := w.Contacts.RecordConversationClaim(w.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](w.contact), Kind: claimKindTheirs,
		Body: subject, ActivityID: ids.UUID(message.Id), Quote: body, Source: "extraction",
	})
	if err != nil {
		t.Fatalf("filing the commitment: %v", err)
	}
	return ids.UUID(claim.Id), ids.UUID(message.Id)
}

// A customer with two open deals: a promise read out of a message filed on one
// deal shows on that deal only. Working at the company is no proof of which
// deal a promise is about.
func TestADealWatchesOnlyThePromisesFiledOnIt(t *testing.T) {
	w := seedDealWatch(t)
	other := w.SeedDeal(t, "CRM project", w.pipeline, w.stage, &w.Rep1)
	w.WsExec(t, `UPDATE deal SET company_id = (SELECT company_id FROM deal WHERE id = $2) WHERE id = $1`, other, w.deal)
	kickoff := w.promiseOn(t, other, "Kick-off", "We will attend the CRM kick-off.")
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	for deal, want := range map[ids.UUID]ids.UUID{w.deal: w.claim, other: kickoff} {
		got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](deal))
		if err != nil {
			t.Fatalf("reading the deal's commitments: %v", err)
		}
		if len(got.Data) != 1 || ids.UUID(got.Data[0].Id) != want || !got.Complete {
			t.Errorf("deal %s watches %+v (complete=%v), want only promise %s", deal, got.Data, got.Complete, want)
		}
	}
}

// The deal page watches what the account's contacts committed to, with the
// words and the message to check it against.
func TestADealWatchesWhatItsCustomerCommittedTo(t *testing.T) {
	w := seedDealWatch(t)
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil {
		t.Fatalf("reading the deal's commitments: %v", err)
	}
	if !got.Complete || len(got.Data) != 1 {
		t.Fatalf("want one commitment and a complete list, got %d (complete=%v)", len(got.Data), got.Complete)
	}
	if row := got.Data[0]; ids.UUID(row.Id) != w.claim || row.ContactName != "Ines Huber" ||
		row.SourceQuote != "We will send the purchase order by Friday." || row.SourceKind != "note" {
		t.Errorf("the row reads %+v, want the claim, its contact, its words and the kind of its source", row)
	}
}

// A commitment a rep dismissed is not watched any more.
func TestADismissedCommitmentLeavesTheDeal(t *testing.T) {
	w := seedDealWatch(t)
	if err := w.Contacts.SettleConversationClaim(w.Admin(), w.claim, "dismissed"); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
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
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil || len(got.Data) != 0 || got.Complete {
		t.Errorf("a hidden commitment reads as %d row(s), complete=%v, err=%v; want none and incomplete",
			len(got.Data), got.Complete, err)
	}
}

// A reader without the contact grant is refused rather than told nothing is owed,
// on a deal with no company as on one with a company.
func TestADealWatchNeedsTheContactGrant(t *testing.T) {
	w := seedDealWatch(t)
	perms := principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "company": {Read: true}, "activity": {Read: true},
			"relationship": {Read: true},
		},
	}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, perms)
	if _, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader with no contact grant: err = %v, want permission denied", err)
	}
	w.WsExec(t, `UPDATE deal SET company_id = NULL WHERE id = $1`, w.deal)
	if _, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader with no contact grant, on a deal with no company: err = %v, want permission denied", err)
	}
}

// A reader without the activity grant is refused too: every row quotes a
// message.
func TestADealWatchNeedsTheActivityGrant(t *testing.T) {
	w := seedDealWatch(t)
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}, "contact": {Read: true}},
	})
	if _, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader with no activity grant: err = %v, want permission denied", err)
	}
}

// The card lists the most urgent commitments and says when there are more.
func TestADealWatchSaysWhenMoreAreOwed(t *testing.T) {
	w := seedDealWatch(t)
	message, err := ids.Parse(w.WsScalar(t, `SELECT source_activity_id::text FROM conversation_claim`))
	if err != nil {
		t.Fatalf("reading the message: %v", err)
	}
	for i := range dealCommitmentsLimit {
		quote := fmt.Sprintf("We will do thing %d.", i)
		w.RecordClaim(w.Admin(), t, contacts.ClaimInput{
			ContactID: ids.From[ids.ContactKind](w.contact), Kind: claimKindTheirs,
			Body: fmt.Sprintf("Commitment %d", i), ActivityID: message,
			Quote: quote, Source: "extraction",
		})
	}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, dealWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil || len(got.Data) != dealCommitmentsLimit || !got.HasMore {
		t.Errorf("an account owing %d commitments reads as %d row(s), has_more=%v, err=%v",
			dealCommitmentsLimit+1, len(got.Data), got.HasMore, err)
	}
}

// noCompanyWatchPerms reads the deal, the contact and the message, and
// neither the company nor any relationship. The watch maps a promise to the
// deal by the message it was read from, so it needs nothing more.
var noCompanyWatchPerms = principal.Permissions{
	RoleKeys: []string{"rep"}, RowScope: principal.RowScopeTeam,
	Objects: map[string]principal.ObjectGrant{
		"deal": {Read: true}, "contact": {Read: true}, "activity": {Read: true},
	},
}

// A reader who may not open the deal's company still watches the promise
// filed on this deal, and the list is complete. The deal itself shows its
// company masked. A promise from the same company filed on another deal is
// not this deal's.
func TestADealWatchNeedsNoCompanyGrant(t *testing.T) {
	w := seedDealWatch(t)
	other := w.SeedDeal(t, "CRM project", w.pipeline, w.stage, &w.Rep1)
	w.WsExec(t, `UPDATE deal SET company_id = (SELECT company_id FROM deal WHERE id = $2) WHERE id = $1`, other, w.deal)
	w.promiseOn(t, other, "Kick-off", "We will attend the CRM kick-off.")
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, noCompanyWatchPerms)

	deal, err := w.Deals.GetDeal(reader, ids.From[ids.DealKind](w.deal), storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	if deal.MaskedFields == nil || !slices.Contains(*deal.MaskedFields, "company_id") {
		t.Fatalf("the deal shows its company to a reader with no company grant: masked=%v", deal.MaskedFields)
	}
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil {
		t.Fatalf("reading the deal's commitments: %v", err)
	}
	if len(got.Data) != 1 || ids.UUID(got.Data[0].Id) != w.claim || !got.Complete {
		t.Errorf("the watch reads %+v (complete=%v), want only this deal's promise and a complete list", got.Data, got.Complete)
	}
}

// The same reader may not open the message a promise was read from. The
// promise is withheld with its quote, and the list says it is incomplete.
func TestADealWatchWithholdsAPromiseWhoseMessageIsHidden(t *testing.T) {
	w := seedDealWatch(t)
	colleague := w.Rep2
	_, source := w.promiseWithSource(t, w.deal, "Pricing", "We will confirm the pricing.")
	if _, err := w.Activities.SetAudience(w.Admin(), ids.From[ids.ActivityKind](source), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: colleague}},
	}); err != nil {
		t.Fatalf("narrowing the message's audience: %v", err)
	}
	reader := w.As(w.Rep1, []ids.UUID{w.Team1}, noCompanyWatchPerms)
	got, err := newDealCommitmentHandlers(w.Pool, func() time.Time { return clocktest.Now(t) }).read(reader, ids.From[ids.DealKind](w.deal))
	if err != nil {
		t.Fatalf("reading the deal's commitments: %v", err)
	}
	if got.Complete || len(got.Data) != 1 || ids.UUID(got.Data[0].Id) != w.claim {
		t.Errorf("the watch reads %+v (complete=%v), want only the readable promise and an incomplete list", got.Data, got.Complete)
	}
	for _, row := range got.Data {
		if row.SourceQuote == "We will confirm the pricing." {
			t.Errorf("the hidden message's quote reached the reader")
		}
	}
}
