// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Asking a colleague for an introduction to somebody at an account, with
// nobody named.
//
// The company's reach card says a colleague has LinkedIn connections there,
// and in which functions. Those connections never consented to appearing in
// this CRM, so the ask names the account and the function and never a contact.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// accountIntroFacts is what an ask for somebody at the account may say.
type accountIntroFacts struct {
	colleague string
	greeting  string
	account   string
	// function is "" when the ask is for anybody there.
	function crmcontracts.LinkedInReachFunction
	lang     textlang.Lang
}

// accountIntroDraft writes the ask from the template. Like the ask for a named
// contact, it reads and writes nothing.
func (s *Service) accountIntroDraft(
	ctx context.Context, companyID ids.CompanyID, active contacts.CustomColumns, req IntroRequest,
) (crmcontracts.CompanyEmailDraft, error) {
	var facts accountIntroFacts
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		facts, err = s.accountIntroFactsFor(ctx, tx, companyID, active, req)
		return err
	})
	if err != nil {
		return crmcontracts.CompanyEmailDraft{}, err
	}
	draft := accountIntroFloor(facts)
	aiWritten := false
	return crmcontracts.CompanyEmailDraft{
		Subject:      draft.subject,
		Body:         draft.body,
		GeneratedBy:  crmcontracts.WrittenByDeterministic,
		AiGenerated:  &aiWritten,
		AiDisclosure: draftfloor.AIProvenanceNoticeFor(false, facts.lang),
		Reasoning: []crmcontracts.AccountDraftReason{{
			Kind:  crmcontracts.AccountDraftReasonKindRelationship,
			Label: fmt.Sprintf("%s has LinkedIn connections at %s", facts.colleague, facts.account),
		}},
	}, nil
}

// accountIntroFactsFor reads the account and the colleague's reach under the
// caller's own scope.
func (s *Service) accountIntroFactsFor(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	active contacts.CustomColumns, req IntroRequest,
) (accountIntroFacts, error) {
	company, err := s.contacts.GetCompanyTx(ctx, tx, companyID, storekit.LiveOnly, active)
	if err != nil {
		return accountIntroFacts{}, err
	}
	reach, err := s.contacts.CompanyReachTx(ctx, tx, companyID)
	if err != nil {
		return accountIntroFacts{}, err
	}
	colleague, found := reachOf(reach, req.ViaUserID)
	if !found {
		// Not a colleague with reach here, or one who withheld it. The two
		// answer the same, because telling them apart would disclose the choice.
		return accountIntroFacts{}, apperrors.ErrNotFound
	}
	facts := accountIntroFacts{colleague: colleague.DisplayName, account: company.DisplayName}
	if req.Function != nil {
		if !holdsFunction(colleague, *req.Function) {
			return accountIntroFacts{}, fmt.Errorf(
				"company360: %s's connections at this account do not work in %s: %w",
				colleague.DisplayName, *req.Function, apperrors.ErrInvalidArgument)
		}
		facts.function = *req.Function
	}
	facts.greeting, err = colleagueGreetingName(ctx, tx, req.ViaUserID)
	if err != nil {
		return accountIntroFacts{}, err
	}
	// No contact has written anything this ask is about, so the language is
	// the reader's own: they and the colleague work in it every day.
	facts.lang = identity.ReaderLanguageForRecord(ctx, tx)
	return facts, nil
}

func reachOf(reach []contacts.CompanyReachColleague, user ids.UserID) (contacts.CompanyReachColleague, bool) {
	for _, colleague := range reach {
		if colleague.UserID == user.UUID {
			return colleague, true
		}
	}
	return contacts.CompanyReachColleague{}, false
}

func holdsFunction(colleague contacts.CompanyReachColleague, function crmcontracts.LinkedInReachFunction) bool {
	for _, held := range colleague.Functions {
		if held.Function == function {
			return true
		}
	}
	return false
}

// accountIntroWording is the ask in each language; the greeting and the
// sign-off are the named-contact ask's own.
var accountIntroWording = struct {
	Subject, Ask, AskIn langcopy.Phrase
}{
	Subject: langcopy.Phrase{
		En: "Could you introduce me to someone at %s?",
		De: "Kannst du mich jemandem bei %s vorstellen?",
		Vi: "Bạn giới thiệu mình với ai đó ở %s được không?",
	},
	Ask: langcopy.Phrase{
		En: "You have connections at %s. Would you be up for introducing me to someone there?",
		De: "Du kennst Leute bei %s. Würdest du mich jemandem dort vorstellen?",
		Vi: "Bạn quen một số người ở %s. Bạn giới thiệu giúp mình với ai đó ở đó nhé?",
	},
	AskIn: langcopy.Phrase{
		En: "You have connections at %s. Would you be up for introducing me to someone there who works in %s?",
		De: "Du kennst Leute bei %s. Würdest du mich jemandem dort vorstellen, der im Bereich %s arbeitet?",
		Vi: "Bạn quen một số người ở %s. Bạn giới thiệu giúp mình với ai đó ở đó làm về %s nhé?",
	},
}

// reachFunctionNames is each function as the ask's sentence names it.
var reachFunctionNames = map[crmcontracts.LinkedInReachFunction]langcopy.Phrase{
	crmcontracts.LinkedInReachFunctionManagement:  {En: "management", De: "Geschäftsführung", Vi: "quản lý"},
	crmcontracts.LinkedInReachFunctionIt:          {En: "IT", De: "IT", Vi: "CNTT"},
	crmcontracts.LinkedInReachFunctionFinance:     {En: "finance", De: "Finanzen", Vi: "tài chính"},
	crmcontracts.LinkedInReachFunctionSales:       {En: "sales", De: "Vertrieb", Vi: "kinh doanh"},
	crmcontracts.LinkedInReachFunctionMarketing:   {En: "marketing", De: "Marketing", Vi: "tiếp thị"},
	crmcontracts.LinkedInReachFunctionHr:          {En: "HR", De: "Personal", Vi: "nhân sự"},
	crmcontracts.LinkedInReachFunctionOperations:  {En: "operations", De: "Betrieb", Vi: "vận hành"},
	crmcontracts.LinkedInReachFunctionProcurement: {En: "procurement", De: "Einkauf", Vi: "mua hàng"},
}

// accountIntroFloor writes the ask. Record values go in as one line through
// draftfloor, for the reasons introFloor gives.
func accountIntroFloor(facts accountIntroFacts) introDraft {
	lang := introLang(facts.lang)
	wording, ok := introTable[lang]
	if !ok {
		wording = introTable[draftfloor.DefaultLang]
	}
	account := draftfloor.OneLine(facts.account)
	greeting := draftfloor.GreetingName(facts.greeting, draftfloor.OneLine(facts.colleague))
	ask := draftfloor.Fill(accountIntroWording.Ask.In(lang), account)
	if name, named := reachFunctionNames[facts.function]; named {
		ask = draftfloor.FillPositional(accountIntroWording.AskIn.In(lang), account, name.In(lang))
	}
	return introDraft{
		subject: draftfloor.Fill(accountIntroWording.Subject.In(lang), account),
		body: strings.Join([]string{
			draftfloor.Fill(wording.greeting, greeting), "", ask, "", wording.sign,
		}, "\n"),
	}
}
