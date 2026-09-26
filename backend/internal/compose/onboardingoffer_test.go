// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a bare yes to Margince's offer may grant, and every way it grants
// nothing: no offer, a stale one, a changed value, a reply that says more than
// yes, and a field the administrator could not save.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// memoryOfferSlot stands where the site_read column does: one slot, replaced
// whole, and the accepted offers the audit rows would name.
type memoryOfferSlot struct {
	slot     *contacts.SiteReadOffer
	accepted []*contacts.SiteReadOffer
	writes   int
	denied   bool
}

func (m *memoryOfferSlot) StandingSiteReadOffer(context.Context, ids.UUID) (*contacts.SiteReadOffer, error) {
	if m.denied {
		return nil, apperrors.ErrPermissionDenied
	}
	return m.slot, nil
}

func (m *memoryOfferSlot) ReplaceSiteReadOffer(_ context.Context, _ ids.UUID, offer, accepted *contacts.SiteReadOffer) error {
	m.slot = offer
	m.writes++
	if accepted != nil {
		m.accepted = append(m.accepted, accepted)
	}
	return nil
}

// offerAdminCtx is a human who may save the company, optionally with fields
// masked from them.
func offerAdminCtx(masked ...string) context.Context {
	masks := make([]principal.FieldMask, 0, len(masked))
	for _, field := range masked {
		masks = append(masks, principal.FieldMask{Object: "company", Field: field})
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys:   []string{"admin"},
			Objects:    map[string]principal.ObjectGrant{"company": {Read: true, Create: true, Update: true}},
			FieldMasks: masks,
		},
	})
}

const offerTurnMessage = "The imprint names Acme Robotics GmbH. Shall I use that as the legal name?"

func legalNameOffer() companyReadOffer {
	return companyReadOffer{Field: fieldLegalName, Value: "Acme Robotics GmbH", SourceIDs: []string{"S1"}}
}

func offerHistory(last ...model.Message) []model.Message {
	history := []model.Message{
		{Role: chatRoleUser, Content: "What is our legal name?"},
		{Role: "assistant", Content: offerTurnMessage},
	}
	return append(history, last...)
}

func recordedLegalNameOffer(draftVersion int) *contacts.SiteReadOffer {
	offer := recordedOffer(legalNameOffer(), offerTurnMessage, draftVersion)
	return &offer
}

func TestAnOfferStandsOnlyOnTheTurnThatMadeIt(t *testing.T) {
	cases := map[string]struct {
		recorded *contacts.SiteReadOffer
		history  []model.Message
		draft    int
		want     bool
	}{
		"the conversation ends on the offering turn": {recordedLegalNameOffer(3), offerHistory(), 3, true},
		"nothing was offered":                        {nil, offerHistory(), 3, false},
		"a later turn came in between": {recordedLegalNameOffer(3), offerHistory(
			model.Message{Role: chatRoleUser, Content: "Hm."},
			model.Message{Role: "assistant", Content: "Anything else?"}), 3, false},
		"the offering turn was reworded": {recordedLegalNameOffer(3), []model.Message{
			{Role: "assistant", Content: "The imprint names Acme AG. Shall I use that as the legal name?"},
		}, 3, false},
		"the dossier was re-read since": {recordedLegalNameOffer(3), offerHistory(), 4, false},
		"no conversation at all":        {recordedLegalNameOffer(3), nil, 3, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := standingOffer(tc.recorded, tc.history, tc.draft)
			if (got != nil) != tc.want {
				t.Fatalf("standingOffer = %+v, want standing %v", got, tc.want)
			}
		})
	}
}

func TestABareYesGrantsExactlyTheOfferedPair(t *testing.T) {
	offer := legalNameOffer()
	granted := companyReadProposedChange{Field: fieldLegalName, Value: " Acme Robotics GmbH "}
	cases := map[string]struct {
		message string
		offer   *companyReadOffer
		change  companyReadProposedChange
		want    bool
	}{
		"a yes to the offer":            {"Yes, that's right.", &offer, granted, true},
		"a German yes to the offer":     {"Ja, genau!", &offer, granted, true},
		"a yes with no offer standing":  {"Yes, that's right.", nil, granted, false},
		"a yes and a different value":   {"Yes", &offer, companyReadProposedChange{Field: fieldLegalName, Value: "Acme Robotics AG"}, false},
		"a yes and another field":       {"Yes", &offer, companyReadProposedChange{Field: fieldDisplayName, Value: "Acme Robotics GmbH"}, false},
		"a yes that corrects the value": {"Yes, but use Acme Robotics AG", &offer, granted, false},
		"a no":                          {"No", &offer, granted, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			authorization := newCompanyChangeAuthorization(tc.message, offerHistory(), "").withStandingOffer(tc.offer)
			if got := authorization.allows(tc.change); got != tc.want {
				t.Fatalf("allows(%+v) = %v, want %v", tc.change, got, tc.want)
			}
		})
	}
}

func TestOnlyAWholeAgreementIsBare(t *testing.T) {
	for _, message := range []string{
		"yes", "Yes!", "Yes, that's right.", "that’s right", "OK", "okay.", "correct", "Right",
		"ja", "Ja genau", "passt", "Stimmt.", "Das passt", "vâng", "Đúng rồi",
	} {
		if !isCompanyChangeConfirmation(message) {
			t.Errorf("%q is a bare agreement and was not recognized", message)
		}
	}
	for _, message := range []string{
		"yes but use Acme AG", "ja, aber nimm Acme AG", "yes, and set the industry too", "no", "not yet",
		"Is that right?", "yes please change the legal name to Acme AG", "Right?", "Correct?", "ok?",
	} {
		if isCompanyChangeConfirmation(message) {
			t.Errorf("%q says more than yes and was taken for bare agreement", message)
		}
	}
}

func TestAnOfferIsGroundedLikeAProposedChange(t *testing.T) {
	known := companyReadEvidenceIndex([]companyReadEvidence{{
		ID: "S1", Kind: "legal_entity", Field: "legal_identity", Value: "Acme Robotics GmbH · HRB 12345",
		Quote: "Acme Robotics GmbH", URL: "https://acme.example/imprint",
	}})
	cited := map[string]struct{}{"S1": {}}
	cases := map[string]struct {
		offers  []companyReadOffer
		sources map[string]struct{}
		want    string
	}{
		"a grounded offer":         {[]companyReadOffer{legalNameOffer()}, cited, ""},
		"two offers":               {[]companyReadOffer{legalNameOffer(), legalNameOffer()}, cited, "more than 1"},
		"a field outside the list": {[]companyReadOffer{{Field: "website", Value: "acme.example", SourceIDs: []string{"S1"}}}, cited, "unsupported field"},
		"an empty value":           {[]companyReadOffer{{Field: fieldLegalName, Value: " ", SourceIDs: []string{"S1"}}}, cited, "empty value"},
		"an unknown source":        {[]companyReadOffer{{Field: fieldLegalName, Value: "Acme Robotics GmbH", SourceIDs: []string{"S9"}}}, cited, "unknown source"},
		"a source the reply omits": {[]companyReadOffer{legalNameOffer()}, map[string]struct{}{}, "absent from reply citations"},
		"no source at all":         {[]companyReadOffer{{Field: fieldLegalName, Value: "Acme Robotics GmbH"}}, cited, "not supported"},
		"a value nobody states":    {[]companyReadOffer{{Field: fieldLegalName, Value: "Acme AG", SourceIDs: []string{"S1"}}}, cited, "not supported"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateCompanyReadOffers(tc.offers, tc.sources, known)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("validateCompanyReadOffers = %v, want accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateCompanyReadOffers = %v, want it to name %q", err, tc.want)
			}
		})
	}
}

func TestEveryAnsweredMessageReplacesTheSlot(t *testing.T) {
	ctx := offerAdminCtx()
	slot := &memoryOfferSlot{}
	read := &contacts.SiteRead{ID: ids.NewV7(), DraftVersion: 2}
	offering := companyReadModelReply{Message: offerTurnMessage, Offers: []companyReadOffer{legalNameOffer()}}

	first, err := beginOfferTurn(ctx, slot, read, nil)
	if err != nil || first.standing != nil {
		t.Fatalf("an empty slot stands %+v (%v)", first.standing, err)
	}
	if err := first.finish(ctx, "What is our legal name?", offering); err != nil {
		t.Fatal(err)
	}
	if slot.slot == nil || slot.slot.Field != fieldLegalName || slot.slot.DraftVersion != 2 {
		t.Fatalf("the offer was not recorded against its draft: %+v", slot.slot)
	}

	yes, err := beginOfferTurn(ctx, slot, read, offerHistory())
	if err != nil || yes.standing == nil {
		t.Fatalf("the offer does not stand on its own turn: %+v (%v)", yes.standing, err)
	}
	accepting := companyReadModelReply{Message: "I'm proposing Acme Robotics GmbH as the legal name.", ProposedChanges: []companyReadProposedChange{
		{Field: fieldLegalName, Value: "Acme Robotics GmbH", Reason: "You agreed.", SourceIDs: []string{"S1"}},
	}}
	if err := yes.finish(ctx, "Yes", accepting); err != nil {
		t.Fatal(err)
	}
	if slot.slot != nil {
		t.Fatalf("an accepted offer survived its answer: %+v", slot.slot)
	}
	if len(slot.accepted) != 1 || slot.accepted[0].Value != "Acme Robotics GmbH" {
		t.Fatalf("the audit names no accepted offer: %+v", slot.accepted)
	}

	again, err := beginOfferTurn(ctx, slot, read, offerHistory())
	if err != nil || again.standing != nil {
		t.Fatalf("a spent offer stands again: %+v (%v)", again.standing, err)
	}
}

func TestAFieldTheAdministratorCannotSaveIsNeverOffered(t *testing.T) {
	read := &contacts.SiteRead{ID: ids.NewV7(), DraftVersion: 1}
	offering := companyReadModelReply{Message: offerTurnMessage, Offers: []companyReadOffer{legalNameOffer()}}

	masked := &memoryOfferSlot{}
	turn, err := beginOfferTurn(offerAdminCtx(fieldLegalName), masked, read, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := turn.finish(offerAdminCtx(fieldLegalName), "What is our legal name?", offering); err != nil {
		t.Fatal(err)
	}
	if masked.slot != nil || masked.writes != 0 {
		t.Fatalf("a masked field was offered: %+v", masked.slot)
	}

	reader := &memoryOfferSlot{denied: true, slot: recordedLegalNameOffer(1)}
	turn, err = beginOfferTurn(offerAdminCtx(), reader, read, offerHistory())
	if err != nil || turn.standing != nil {
		t.Fatalf("a caller without save authority holds an offer: %+v (%v)", turn.standing, err)
	}
	if err := turn.finish(offerAdminCtx(), "Yes", offering); err != nil || reader.writes != 0 {
		t.Fatalf("a caller without save authority wrote the slot (%v)", err)
	}
}

// The onboarding acts share the reply schema and edit nothing, so an offer
// from one is refused like a proposed change.
func TestAnOnboardingActMayNotOffer(t *testing.T) {
	reply := `{"kind":"answer","message":"Shall I?","proposed_changes":[],` +
		`"offers":[{"field":"legal_name","value":"Acme","source_ids":[]}],"source_ids":[]}`
	if err := validateOnboardingActReply("voice", reply); err == nil {
		t.Fatal("an act's offer was accepted")
	}
}

// A bare yes is decided by whether the previous reply offered anything, so
// both conversations state it in every request, null included: an omitted
// field is one the model never saw, and a yes to a plain question then reads
// as accepting the value the question named.
func TestEveryCompanyConversationStatesWhetherAnOfferStands(t *testing.T) {
	offer := &companyReadOffer{Field: "display_name", Value: "Acme", SourceIDs: []string{"S1"}}
	build := map[string]func(*companyReadOffer) (model.Request, error){
		"dossier conversation": func(standing *companyReadOffer) (model.Request, error) {
			return companyReadAnswerRequest("Yes.", nil, nil, standing)
		},
		"setup conversation": func(standing *companyReadOffer) (model.Request, error) {
			return onboardingCompanyAnswerRequest("Yes.", nil, onboardingConversationContext{PreviousOffer: standing}, "en", nil)
		},
	}
	for site, request := range build {
		for standing, want := range map[*companyReadOffer]string{
			nil:   `"your_previous_offer":null`,
			offer: `"your_previous_offer":{"field":"display_name","value":"Acme"`,
		} {
			req, err := request(standing)
			if err != nil {
				t.Fatalf("%s: building the request: %v", site, err)
			}
			if !strings.Contains(req.Messages[0].Content, want) {
				t.Errorf("%s: the application state does not carry %s:\n%s", site, want, req.Messages[0].Content)
			}
		}
	}
}
