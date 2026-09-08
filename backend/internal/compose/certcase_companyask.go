// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/company_ask — the company view's prepared
// questions.
//
// It shares everything with summarize/company_brief except the request: the same
// account fixture, the same minted-id discipline, and the same production
// grounding filter deciding which sentences survive. What differs is the one
// thing this site adds — the answer must be grounded in the records the ASKED
// question is about, not merely in the account somewhere.
//
// That is why the scenario names both a question and the labelled records a
// correct answer cites. A `whats_open` answer that cites only last month's
// email is grounded and wrong, and a case that checked grounding alone would
// score it as correct.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// companyAskFixture is one account plus the question asked about it. The account
// half is the brief's fixture, unchanged: two shapes would let the two sites
// drift into certifying different accounts.
type companyAskFixture struct {
	companyBriefFixture
	Question crmcontracts.CompanyQuestion `json:"question"`
}

type companyAskCases struct{}

func (companyAskCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskSummarize, Variant: "company_ask", Kind: ai.SiteKindOneShot}
}

// Prepare turns one account, one prepared question and the records a correct
// answer must cite into a runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (companyAskCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f companyAskFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/company_ask: the fixture is not the shape this site takes: %w", err)
	}
	// The question runs through the production validator, so a scenario cannot
	// certify a question the endpoint would refuse to answer.
	question, err := companybrief.ParseQuestion(f.Question)
	if err != nil {
		return nil, fmt.Errorf("summarize/company_ask: %w", err)
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"summarize/company_ask: the expected answer is not a list of record labels the answer must cite: %w", err)
	}
	in, label, err := companyBriefInput(f.companyBriefFixture)
	if err != nil {
		return nil, fmt.Errorf("summarize/company_ask: %w", err)
	}
	if err := refuseUngroundableBrief(want, label); err != nil {
		return nil, fmt.Errorf("summarize/company_ask: %w", err)
	}
	return &companyBriefCase{
		site: "summarize/company_ask",
		request: func(in companybrief.Input) model.Request {
			// English, pinned, rather than the installation's base language: a
			// certification record grades a fixed corpus, and a score that moved
			// with a settings row would not be comparable between installations.
			return companybrief.AskRequest(question, in, string(textlang.English))
		},
		in: in, companyID: ids.NewV7().String(), label: label, expected: want,
	}, nil
}
