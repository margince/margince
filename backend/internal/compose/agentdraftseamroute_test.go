// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askedEngine records which drafting engine was asked, and about which record,
// and refuses so the draft stops there.
type askedEngine struct {
	engine string
	record ids.UUID
}

var errEngineAsked = errors.New("engine asked")

func (a *askedEngine) asked(engine string, record ids.UUID) error {
	a.engine, a.record = engine, record
	return errEngineAsked
}

type askedContactView struct{ *askedEngine }

func (v askedContactView) AssembleScoped(_ context.Context, id ids.ContactID, _ contact360.AssembleOptions) (crmcontracts.Contact360, error) {
	return crmcontracts.Contact360{}, v.asked("contact", id.UUID)
}

type askedCompanyView struct{ *askedEngine }

func (v askedCompanyView) AssembleScoped(_ context.Context, id ids.CompanyID, _ company360.AssembleOptions) (crmcontracts.Company360, error) {
	return crmcontracts.Company360{}, v.asked("account", id.UUID)
}

type askedLeads struct{ *askedEngine }

func (l askedLeads) GetLead(_ context.Context, id ids.LeadID, _ storekit.ArchivedFilter) (crmcontracts.Lead, error) {
	return crmcontracts.Lead{}, l.asked("lead", id.UUID)
}

func (askedLeads) ForLead(context.Context, ids.LeadID) ([]crmcontracts.Activity, error) {
	return nil, nil
}

func draftingAgent() context.Context {
	seat := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: seat, OnBehalfOf: seat,
		Scopes: principal.NewScopeSet(principal.ScopeDraft),
	})
}

func TestDraftEmailAsksTheEngineTheComposerWouldAsk(t *testing.T) {
	contact, lead, company := ids.NewV7(), ids.NewV7(), ids.NewV7()
	link := func(kind string, id ids.UUID) agents.RecordLink {
		return agents.RecordLink{EntityType: kind, EntityID: id}
	}

	for name, want := range map[string]struct {
		links  []agents.RecordLink
		engine string
		record ids.UUID
	}{
		"a contact alone":        {[]agents.RecordLink{link("contact", contact)}, "contact", contact},
		"a contact at a company": {[]agents.RecordLink{link("contact", contact), link("company", company)}, "account", company},
		"a lead":                 {[]agents.RecordLink{link("lead", lead)}, "lead", lead},
	} {
		asked := &askedEngine{}
		engines := &firstMessageEngines{
			contact: contactdraft.NewService(askedContactView{asked}, nil),
			account: accountdraft.NewService(askedCompanyView{asked}, nil),
			lead:    leaddraft.NewService(askedLeads{asked}, askedLeads{asked}, nil),
		}
		_, err := commsAdapter{firstDrafts: engines}.DraftCompanyEmail(draftingAgent(), want.links, "follow up")
		if !errors.Is(err, errEngineAsked) {
			t.Fatalf("%s: got %v, want the engine's own answer", name, err)
		}
		if asked.engine != want.engine || asked.record != want.record {
			t.Errorf("%s asked the %s engine about %s, want the %s engine about %s",
				name, asked.engine, asked.record, want.engine, want.record)
		}
	}
}

func TestDraftEmailRefusesBeforeAnyEngineIsAsked(t *testing.T) {
	contact := []agents.RecordLink{{EntityType: "contact", EntityID: ids.NewV7()}}
	var bad *agents.BadArgsError
	if _, err := (commsAdapter{}).DraftCompanyEmail(draftingAgent(), nil, "x"); !errors.As(err, &bad) {
		t.Errorf("no links: got %v, want a refusal naming the arguments", err)
	}
	if _, err := (commsAdapter{}).DraftCompanyEmail(draftingAgent(), contact, "x"); err == nil {
		t.Error("a surface with no drafting engine wired drafted anyway")
	}
}

func TestAFirstDraftCarriesTheEnginesAddressAndAIMarking(t *testing.T) {
	ai, notice := true, "Written by a model."
	to := []openapi_types.Email{"dana@example.test"}
	got := firstDraftOf(crmcontracts.CompanyEmailDraft{
		Subject: "Hello", Body: "Hi Dana,", To: &to, AiGenerated: &ai, AiDisclosure: &notice,
	})
	if got.Subject != "Hello" || got.Body != "Hi Dana," || len(got.To) != 1 || got.To[0] != "dana@example.test" ||
		!got.AIGenerated || got.AIDisclosure != notice {
		t.Errorf("the first draft read %+v, want the engine's subject, body, address and AI marking", got)
	}
	if plain := firstDraftOf(crmcontracts.CompanyEmailDraft{Subject: "Hello"}); plain.AIGenerated || plain.To != nil {
		t.Errorf("a floor draft read %+v, want no address and no AI marking", plain)
	}
}

// The api role binds each engine's model lane after the tool registry is built,
// so the option must replace the very engine draft_email reads, not only the
// HTTP route's.
func TestTheDraftOptionsRebindTheEnginesDraftEmailReads(t *testing.T) {
	s := &Server{}
	for _, bind := range []Option{WithContactDraft(nil), WithAccountDraft(nil), WithLeadDraft(nil)} {
		bind(s, nil)
	}
	if s.firstDrafts.contact == nil || s.firstDrafts.account == nil || s.firstDrafts.lead == nil {
		t.Errorf("after the options, draft_email reads %+v, want all three engines bound", s.firstDrafts)
	}
}
