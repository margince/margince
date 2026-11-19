// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A scheduled message comes back out exactly as it went in.
//
// The freeze/thaw pair is the one place a send's own description can narrow
// without anything failing: a field added to SendEmailInput and not to
// scheduledPayload is dropped silently, and the send still goes — just
// describing itself as less than it was. That failed safe for the context
// fields (an empty claim records NULL rather than a false claim) and would not
// have for marketing_purpose, whose whole job is to scope a grant to one topic:
// a send scheduled for one minute from now would have arrived with no purpose
// to check against.
//
// So this compares field by field through reflection rather than asserting the
// handful somebody remembered. The next field added to SendEmailInput fails
// here until it is carried.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// A fully-populated input survives the round trip with every field intact.
func TestAScheduledSendCarriesEveryFieldOfItsInput(t *testing.T) {
	in := SendEmailInput{
		Recipients:       []string{"buyer@example.test", "cc@example.test"},
		Cc:               []string{"cc@example.test"},
		Bcc:              []string{"blind@example.test"},
		Subject:          "Your quote",
		Body:             "As discussed.",
		HTMLBody:         "<p>As discussed.</p>",
		AttachmentIDs:    []ids.UUID{ids.NewV7()},
		ConsentPurpose:   "marketing_email",
		DraftRef:         "draft-7",
		Context:          commsauthz.CategoryMarketing,
		MarketingPurpose: "newsletter",
		OperatorReason:   "they asked at the fair",
		Evidence: commsauthz.Evidence{
			ActivityID:     ids.NewV7(),
			DealID:         ids.NewV7(),
			InvoiceID:      ids.NewV7(),
			ContractID:     ids.NewV7(),
			ConsentEventID: ids.NewV7(),
			BasisID:        ids.NewV7(),
			RecipientCountries: []commsauthz.CountryOverride{
				{Address: "buyer@example.test", Country: "at"},
				{Address: "blind@example.test"},
			},
		},
		SendingEntity:  ids.NewV7(),
		FromConnection: ids.NewV7(),
	}

	out, err := freezePayload(in).thaw()
	if err != nil {
		t.Fatalf("thawing a frozen send: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("a scheduled send came back changed:\n frozen: %+v\n thawed: %+v", in, out)
	}
}

// Every field of the input is actually SET by the fixture above. Without this,
// a new field would be zero on both sides, DeepEqual would pass, and the test
// meant to catch a dropped field would be the thing hiding it.
func TestTheRoundTripFixtureLeavesNoFieldUnset(t *testing.T) {
	in := SendEmailInput{
		Recipients:       []string{"buyer@example.test"},
		Cc:               []string{"cc@example.test"},
		Bcc:              []string{"blind@example.test"},
		Subject:          "Your quote",
		Body:             "As discussed.",
		HTMLBody:         "<p>As discussed.</p>",
		AttachmentIDs:    []ids.UUID{ids.NewV7()},
		ConsentPurpose:   "marketing_email",
		DraftRef:         "draft-7",
		Context:          commsauthz.CategoryMarketing,
		MarketingPurpose: "newsletter",
		OperatorReason:   "they asked at the fair",
		Evidence:         commsauthz.Evidence{ActivityID: ids.NewV7()},
		SendingEntity:    ids.NewV7(),
		FromConnection:   ids.NewV7(),
	}
	v := reflect.ValueOf(in)
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if notFrozen[name] {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("SendEmailInput.%s is zero in the round-trip fixture, so the round trip proves nothing about it",
				name)
		}
	}
}

// notFrozen names the fields a freeze deliberately DROPS, so the census above
// does not demand a round trip for something that must not make one.
//
// Each entry is a claim that the field describes the CALL rather than the
// message, and a test below holds every one of them.
var notFrozen = map[string]bool{
	// Where this send was resumed FROM. A held message that is refused again is
	// frozen afresh and gets its own held row; carrying the old intent into the
	// new payload would make the second row point at the first, so a resume
	// would reach for a message that is already gone — and the decision
	// standing over the predecessor would be found for a message nobody
	// acknowledged.
	"ResumingIntentID": true,
	// The draft the message was composed in. The schedule discards it in the
	// transaction that freezes the message, so a thawed id names nothing.
	"MailDraftID": true,
}

// THE DROPPED FIELDS ARE ACTUALLY DROPPED. Without this the exemption above
// would be a way to stop testing a field rather than a statement about it: a
// field listed here and then quietly frozen would round-trip, and nothing would
// say the claim had stopped being true.
func TestAFreezeCarriesNothingAboutTheCallThatMadeIt(t *testing.T) {
	in := SendEmailInput{
		Recipients:       []string{"buyer@example.test"},
		Subject:          "Your quote",
		Body:             "As discussed.",
		ResumingIntentID: ids.NewV7(),
		MailDraftID:      ids.NewV7(),
	}
	out, err := freezePayload(in).thaw()
	if err != nil {
		t.Fatalf("thawing a frozen send: %v", err)
	}
	if !out.ResumingIntentID.IsZero() {
		t.Error("a freeze carried the intent this send was resumed from — a message refused " +
			"again would be held pointing at the message it replaced, and a decision taken " +
			"about that predecessor would be found for one nobody acknowledged")
	}
	if !out.MailDraftID.IsZero() {
		t.Error("a freeze carried the draft the message was composed in, which the schedule already discarded")
	}
}

// An unnamed record stays unnamed. Rendering a zero id as text would put the
// all-zeroes UUID on the row, which reads as a record that exists.
func TestAnUnnamedRecordDoesNotBecomeAZeroID(t *testing.T) {
	frozen := freezePayload(SendEmailInput{Subject: "Hello"})
	if !reflect.DeepEqual(frozen.Evidence, frozenEvidence{}) {
		t.Errorf("evidence = %+v, want every field empty when the caller named nothing", frozen.Evidence)
	}
	out, err := frozen.thaw()
	if err != nil {
		t.Fatalf("thawing: %v", err)
	}
	if !reflect.DeepEqual(out.Evidence, commsauthz.Evidence{}) {
		t.Errorf("evidence = %+v, want zero after a round trip that named nothing", out.Evidence)
	}
}

// The row keeps a cleared country apart from no statement, through the JSON
// the column holds.
func TestAFrozenRowKeepsAClearedCountryApartFromNone(t *testing.T) {
	raw, err := json.Marshal(freezePayload(SendEmailInput{
		Recipients: []string{"buyer@example.test", "other@example.test"},
		Evidence: commsauthz.Evidence{RecipientCountries: []commsauthz.CountryOverride{
			{Address: "buyer@example.test"},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"recipient_countries":[{"address":"buyer@example.test","country":null}]`) {
		t.Fatalf("payload = %s, want the cleared country stored as null", raw)
	}
	var back scheduledPayload
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	out, err := back.thaw()
	if err != nil {
		t.Fatal(err)
	}
	stated := out.Evidence.RecipientCountries
	if len(stated) != 1 || stated[0].Address != "buyer@example.test" || !stated[0].Cleared() {
		t.Errorf("thawed countries = %+v, want one cleared entry for buyer and none for other", stated)
	}
}

// A version-2 row written before countries were frozen still fires, with no
// country stated: the recipients keep the record's.
func TestAPayloadWrittenBeforeCountriesThawsWithNone(t *testing.T) {
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(`{"recipients":["buyer@example.test"],"subject":"Hi","body":"Hello",
		"consent_purpose":"","evidence":{"deal_id":"`+ids.NewV7().String()+`"}}`), &payload); err != nil {
		t.Fatal(err)
	}
	out, err := payload.thaw()
	if err != nil {
		t.Fatalf("an older payload no longer thaws: %v", err)
	}
	if out.Evidence.RecipientCountries != nil || out.Evidence.DealID.IsZero() {
		t.Errorf("evidence = %+v, want the deal and no stated country", out.Evidence)
	}
}

// A reserved category cannot be restored from a scheduled payload.
//
// It can never legitimately be in one: the door that writes the payload refuses
// it, and freezePayload has a single caller behind that door. Refusing it again
// at the restore costs a live send nothing and removes the assumption — the
// exemption this shape holds in the send-door gate rests on there being exactly
// one payload writer, and nothing fails today if somebody adds a second.
func TestAReservedCategoryCannotBeRestoredFromAScheduledPayload(t *testing.T) {
	for _, c := range commsauthz.Categories() {
		if !c.ServesTheSubject() {
			continue
		}
		payload := freezePayload(SendEmailInput{Subject: "Anything"})
		payload.Context = string(c)
		if _, err := payload.thaw(); err == nil {
			t.Errorf("a scheduled payload restored %q, which no send may claim", c)
		}
	}
}

// And an ordinary category still restores, or the refusal above would be
// indistinguishable from a thaw that rejects every category.
func TestAnOrdinaryCategoryStillRestores(t *testing.T) {
	payload := freezePayload(SendEmailInput{
		Subject: "Anything", Context: commsauthz.CategoryReplyToInbound,
	})
	out, err := payload.thaw()
	if err != nil {
		t.Fatalf("an ordinary category was refused at restore: %v", err)
	}
	if out.Context != commsauthz.CategoryReplyToInbound {
		t.Errorf("context = %q, want reply_to_inbound", out.Context)
	}
}
