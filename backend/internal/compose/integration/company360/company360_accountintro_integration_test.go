// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// Asking a colleague for an introduction to somebody at an account, from the
// reach card. The colleague must have LinkedIn reach there. The draft names
// the account and never one of those connections.

import (
	"errors"
	"strings"
	"testing"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectedAt gives a colleague one LinkedIn connection at an account, with
// the job title the export carries.
func connectedAt(t *testing.T, e *integration.Env, owner, company ids.UUID, name, title string) {
	t.Helper()
	e.WsExec(t, `
		INSERT INTO linkedin_connection
		  (owner_user_id, full_name, normalized_name, position, company_name,
		   normalized_company, matched_company_id, match_status, source)
		VALUES ($1, $2, lower($2), $3, 'Brandt GmbH', 'brandt gmbh', $4, 'suggested', 'csv_export')`,
		owner, name, title, company)
}

func TestAnAccountAskNamesTheFunctionAndNoConnection(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _ := seedRoleDeal(t, e)
	connectedAt(t, e, e.Rep2, company, "Abbas Fawaz", "Head of IT")
	connectedAt(t, e, e.Rep2, company, "Bea Hoffmann", "IT-Leiterin")

	it := crmcontracts.LinkedInReachFunctionIt
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)
	draft, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{ViaUserID: ids.From[ids.UserKind](e.Rep2), Function: &it})
	if err != nil {
		t.Fatalf("an ask at the account was refused: %v", err)
	}
	if !strings.Contains(draft.Body, "Brandt GmbH") || !strings.Contains(draft.Body, "works in IT") {
		t.Errorf("the draft does not ask for somebody in IT at the account:\n%s", draft.Body)
	}
	for _, connection := range []string{"Abbas", "Bea", "Head of IT", "IT-Leiterin"} {
		if strings.Contains(draft.Subject+draft.Body, connection) {
			t.Errorf("the draft names %q, one of the colleague's connections:\n%s", connection, draft.Body)
		}
	}
	if draft.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Errorf("the draft is credited to %q, want the template", draft.GeneratedBy)
	}
}

func TestAnAccountAskRefusesAColleagueWithNoReachThere(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _ := seedRoleDeal(t, e)
	connectedAt(t, e, e.Rep2, company, "Abbas Fawaz", "Head of IT")

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)
	_, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{ViaUserID: ids.From[ids.UserKind](e.Rep3)})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an ask of a colleague with no reach answered %v, want not found", err)
	}
	// The same caller asking the colleague who has reach is drafted, so the
	// refusal above is about the colleague rather than about the caller.
	if _, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{ViaUserID: ids.From[ids.UserKind](e.Rep2)}); err != nil {
		t.Fatalf("an ask of the colleague with reach was refused: %v", err)
	}
}

// A function with one connection is not on the reach answer, so an ask for it
// would name the one contact behind that title.
func TestAnAccountAskRefusesAFunctionTheReachAnswerDoesNotShow(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _ := seedRoleDeal(t, e)
	connectedAt(t, e, e.Rep2, company, "Abbas Fawaz", "Head of IT")
	connectedAt(t, e, e.Rep2, company, "Bea Hoffmann", "IT-Leiterin")
	connectedAt(t, e, e.Rep2, company, "Cem Yilmaz", "Head of HR")

	hr := crmcontracts.LinkedInReachFunctionHr
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)
	_, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{ViaUserID: ids.From[ids.UserKind](e.Rep2), Function: &hr})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("an ask for a function held by one connection answered %v, want invalid", err)
	}
}

func TestAnAccountAskRefusesTheReaderAsTheirOwnIntroducer(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _ := seedRoleDeal(t, e)
	connectedAt(t, e, e.Rep1, company, "Abbas Fawaz", "Head of IT")

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)
	_, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{ViaUserID: ids.From[ids.UserKind](e.Rep1)})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a reader drafted an ask of themselves: %v", err)
	}
}

// A function narrows an ask at the account. With a contact named, it would be
// dropped without a word, and the reader would send an ask that says less than
// they chose.
func TestAFunctionWithANamedContactIsRefused(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _ := seedRoleDeal(t, e)
	contact := e.SeedContact(t, "Ute Sommer", nil)

	it := crmcontracts.LinkedInReachFunctionIt
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)
	_, err := svc.IntroRequestDraft(reader, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{
			ContactID: new(ids.From[ids.ContactKind](contact)),
			ViaUserID: ids.From[ids.UserKind](e.Rep2),
			Function:  &it,
		})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a function with a named contact answered %v, want invalid", err)
	}
}
