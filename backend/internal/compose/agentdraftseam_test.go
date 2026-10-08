// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// oneContactView answers the same 360 for every caller, so any difference in
// what reaches the model comes from the door, not from the data.
type oneContactView struct{ contact crmcontracts.Contact }

func (v oneContactView) AssembleScoped(context.Context, ids.ContactID, contact360.AssembleOptions) (crmcontracts.Contact360, error) {
	return crmcontracts.Contact360{Contact: v.contact}, nil
}

// unansweredLane keeps every request and answers none, so the engine falls to
// its floor after the request it built has been seen.
type unansweredLane struct{ requests []model.Request }

func (l *unansweredLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.requests = append(l.requests, req)
	return model.Response{}, errors.New("recorded, not answered")
}

func TestDraftEmailBuildsTheRequestTheContactComposerBuilds(t *testing.T) {
	contactID := ids.NewV7()
	first, last := "Marta", "Kowalska"
	address := openapi_types.Email("marta@example.test")
	view := oneContactView{contact: crmcontracts.Contact{
		Id: openapi_types.UUID(contactID), FullName: "Marta Kowalska",
		FirstName: &first, LastName: &last, PrimaryEmail: &address,
	}}
	at := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	engine := func(lane *unansweredLane) *contactdraft.Service {
		return contactdraft.NewService(view, lane).
			WithEnvelope(draftfloor.NewResolver().WithClock(func() time.Time { return at }))
	}
	const intent = "follow up on Tuesday's demo"
	seat := ids.NewV7()

	web := &unansweredLane{}
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	req := httptest.NewRequestWithContext(human, http.MethodPost, "/contacts/x/draft-email",
		strings.NewReader(`{"intent":"`+intent+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	contactdraft.NewHandlers(engine(web)).DraftContactEmail(rec, req, openapi_types.UUID(contactID))
	if rec.Code != http.StatusOK {
		t.Fatalf("the composer's route answered %d: %s", rec.Code, rec.Body.String())
	}

	tool := &unansweredLane{}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: seat, OnBehalfOf: seat,
		Scopes: principal.NewScopeSet(principal.ScopeDraft),
	})
	message, err := firstMessageOf([]agents.RecordLink{{EntityType: "contact", EntityID: contactID}})
	if err != nil {
		t.Fatalf("firstMessageOf: %v", err)
	}
	engines := &firstMessageEngines{contact: engine(tool)}
	if _, _, err := engines.draft(agent, message, intent); err != nil {
		t.Fatalf("draft_email's engine: %v", err)
	}

	if len(web.requests) == 0 {
		t.Fatal("the composer's route asked the model nothing")
	}
	if got, want := unfenced(tool.requests), unfenced(web.requests); got != want {
		t.Errorf("draft_email built a different request than the composer:\nweb:  %s\ntool: %s", want, got)
	}
}

// fenceMarker is the per-call nonce of the untrusted-data fence, the one part
// of a request minted fresh on every call by design.
var fenceMarker = regexp.MustCompile(`untrusted-[0-9a-f-]{36}`)

func unfenced(requests []model.Request) string {
	return fenceMarker.ReplaceAllString(fmt.Sprintf("%+v", requests), "untrusted-FENCE")
}

func TestDraftEmailLinksNameOneRecipient(t *testing.T) {
	contact, lead, company := ids.NewV7(), ids.NewV7(), ids.NewV7()
	link := func(kind string, id ids.UUID) agents.RecordLink {
		return agents.RecordLink{EntityType: kind, EntityID: id}
	}

	for name, links := range map[string][]agents.RecordLink{
		"a company alone":                  {link("company", company)},
		"two recipients":                   {link("contact", contact), link("lead", lead)},
		"two companies":                    {link("contact", contact), link("company", company), link("company", ids.NewV7())},
		"an activity beside a contact":     {link("contact", contact), link("activity", ids.NewV7())},
		"an unknown type beside a contact": {link("contact", contact), link("invoice", ids.NewV7())},
		"a deal and no recipient":          {link("deal", ids.NewV7())},
		"two deals": {
			link("contact", contact), link("company", company),
			link("deal", ids.NewV7()), link("deal", ids.NewV7()),
		},
		"two projects":                    {link("contact", contact), link("project", ids.NewV7()), link("project", ids.NewV7())},
		"a lead with a company":           {link("lead", lead), link("company", company)},
		"a lead with a deal":              {link("lead", lead), link("deal", ids.NewV7())},
		"a lead with a project":           {link("lead", lead), link("project", ids.NewV7())},
		"a deal on a contact, no company": {link("contact", contact), link("deal", ids.NewV7())},
	} {
		var bad *agents.BadArgsError
		if _, err := firstMessageOf(links); !errors.As(err, &bad) {
			t.Errorf("%s: got %v, want a refusal naming the arguments", name, err)
		}
	}

	for name, links := range map[string][]agents.RecordLink{
		"a lead alone":                    {link("lead", lead)},
		"a contact with a project":        {link("contact", contact), link("project", ids.NewV7())},
		"a deal at the contact's company": {link("contact", contact), link("company", company), link("deal", ids.NewV7())},
	} {
		if _, err := firstMessageOf(links); err != nil {
			t.Errorf("%s: refused with %v, want an engine that reads every link", name, err)
		}
	}

	message, err := firstMessageOf([]agents.RecordLink{link("company", company), link("contact", contact)})
	if err != nil {
		t.Fatalf("a contact at a company: %v", err)
	}
	if message.recipient.ID != contact || message.recipient.Type != crmcontracts.MailDraftAnchorTypeContact || message.company != company {
		t.Errorf("a contact at a company read as %+v, want the contact as recipient and the company beside it", message)
	}
}
