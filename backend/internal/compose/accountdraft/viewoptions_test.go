// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordingAccount answers one view and keeps the options it was asked with.
type recordingAccount struct {
	view  crmcontracts.Company360
	asked []company360.AssembleOptions
}

func (a *recordingAccount) AssembleScoped(_ context.Context, _ ids.CompanyID, opts company360.AssembleOptions) (crmcontracts.Company360, error) {
	a.asked = append(a.asked, opts)
	return a.view, nil
}

// The draft reads the account narrowed to the recipient's own conversations.
// The SQL behind the narrowing is proved against a database in
// TestAnAccountDraftReadsOnlyTheRecipientsOwnConversation.
func TestTheDraftReadsOnlyTheRecipientsConversations(t *testing.T) {
	t.Parallel()
	recipient := ids.NewV7()
	project := ids.From[ids.ProjectKind](ids.NewV7())
	account := &recordingAccount{}
	_, _, err := NewService(account, nil).Draft(humanCaller(), ids.From[ids.CompanyKind](ids.NewV7()),
		Request{ContactID: recipient.String(), ProjectID: &project})
	if err == nil {
		t.Fatal("a view with no contacts drafted to somebody")
	}
	if len(account.asked) != 1 {
		t.Fatalf("the draft read the account %d times, want once", len(account.asked))
	}
	opts := account.asked[0]
	if opts.PartyContactID == nil || opts.PartyContactID.UUID != recipient {
		t.Errorf("the timeline is narrowed to %v, want the recipient %s", opts.PartyContactID, recipient)
	}
	if !opts.WrittenByHumanOnly || opts.ProjectID == nil || *opts.ProjectID != project {
		t.Errorf("the read lost its other narrowings: %+v", opts)
	}
}

// A contact id that names nobody is refused before the account is read.
func TestAMalformedRecipientIsRefusedBeforeTheRead(t *testing.T) {
	t.Parallel()
	account := &recordingAccount{}
	_, _, err := NewService(account, nil).Draft(humanCaller(), ids.From[ids.CompanyKind](ids.NewV7()),
		Request{ContactID: "not-a-contact"})
	detailed, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || detailed.Status != 422 {
		t.Fatalf("a malformed contact id answered %v, want a 422", err)
	}
	if len(account.asked) != 0 {
		t.Error("the account was read for a recipient that names nobody")
	}
}

// humanCaller is a signed-in member, which is who presses Write email.
func humanCaller() context.Context {
	seat := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
}
