// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company context a certified prompt carries: production prepends it to
// every call of a task whose contract declares scopes, read from the
// installation's own company record. The lane has no database, so it serves one
// fixed demo company instead, through the same provider and the same assembler
// a production read goes through. What a record then measures is the prompt an
// installation with a company record sends.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// certCompanyWebsite is the demo company's domain, on the reserved example TLD.
var certCompanyWebsite = "northwind-ledger.example"

// certCompany is the demo installation's own company. It sells subscription
// software with onboarding services, which is what the drafting and offer
// scenarios assume of the sender, and it states no tier, price or feature a
// scenario's rubric could read as invented.
var certCompany = contacts.Company{
	CompanyID:     ids.From[ids.CompanyKind](ids.MustParse("0190d7a4-6c1e-7a3b-9f2d-4c5e6f7a8b9c")),
	DisplayName:   "Northwind Ledger",
	CompanySource: contacts.CompanySourceHuman,
	Website:       &certCompanyWebsite,
	ProfileFields: []contacts.CompanyProfileField{
		{Field: "industry", Value: "B2B software", Source: contacts.CompanySourceHuman},
		{Field: "icp", Value: "Mid-sized manufacturers and distributors that sell to other businesses", Source: contacts.CompanySourceHuman},
		{Field: "value_proposition", Value: "One place for a sales team to see every account, deal and promise", Source: contacts.CompanySourceHuman},
		{Field: "customer_pains", Value: "Follow-ups lost between inboxes and spreadsheets", Source: contacts.CompanySourceHuman},
		{Field: "buying_center", Value: "Head of Sales, with sign-off from the managing director", Source: contacts.CompanySourceHuman},
		{Field: "sales_motion", Value: "A demo call, then a paid pilot", Source: contacts.CompanySourceHuman},
		{Field: "offer_summary", Value: "A subscription CRM with onboarding consulting", Source: contacts.CompanySourceHuman},
	},
	Facts: []contacts.CompanyFact{
		{Category: "offering", Field: contacts.FactService, Value: "Onboarding consulting", Source: contacts.CompanySourceHuman},
		{Category: "market", Field: contacts.FactServedIndustry, Value: "Manufacturing", Source: contacts.CompanySourceHuman},
		{Category: "market", Field: contacts.FactGeography, Value: "Germany, Austria and Switzerland", Source: contacts.CompanySourceHuman},
		{Category: "signal", Field: contacts.FactCertification, Value: "ISO 27001", Source: contacts.CompanySourceHuman},
	},
}

// certCompanyReader stands where the store's read stands: it assembles the
// scopes the provider asks for from the demo company, at a fixed instant so two
// runs of one case send one prompt.
type certCompanyReader struct{}

func (certCompanyReader) GetCompanyContext(_ context.Context, requested []contacts.CompanyContextScope, _ contacts.OfferingSelector) (contacts.CompanyContext, error) {
	return contacts.AssembleCompanyContext(certCompany, requested, time.Time{}), nil
}

// withCertCompanyContext prepares req as the model path prepares every call of
// task, the demo company being the installation's own.
func withCertCompanyContext(ctx context.Context, task ai.Task, req model.Request) (model.Request, error) {
	prepared, err := newCompanyContextProvider(certCompanyReader{}).Prepare(ctx, task, req)
	if err != nil {
		return model.Request{}, fmt.Errorf("%s: %w", task, err)
	}
	return prepared, nil
}

// certContextBrain is the brain a drafter that calls the model several times
// writes through: like routerBrain, it prepares each call before it is sent, so
// a recorder below it records the request the model is actually shown.
type certContextBrain struct {
	task ai.Task
	next aitasks.Completer
}

func (b certContextBrain) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	prepared, err := withCertCompanyContext(ctx, b.task, req)
	if err != nil {
		return model.Response{}, err
	}
	return b.next.Complete(ctx, prepared)
}
