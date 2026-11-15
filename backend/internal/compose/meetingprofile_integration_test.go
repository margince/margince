// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingProfileReply is a reading that states no next step and one way the
// customer's company sells, cited to line 1.
func meetingProfileReply(t *testing.T, field, value string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"proposals": []any{},
		"company_profile": []map[string]any{{
			"field": field, "value": value, "source_lines": []int{1}, "confidence": 0.8,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// customerAt links an employee of a new company to the transcript.
func (e *transcriptEnv) customerAt(t *testing.T, company ids.UUID, name string) {
	t.Helper()
	contact := e.SeedContact(t, name, &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	e.linkTranscriptContact(t, contact)
}

func (e *transcriptEnv) profileField(t *testing.T, company ids.UUID, field string) (crmcontracts.CompanyProfileField, bool) {
	t.Helper()
	fields, err := e.Contacts.ListCompanyProfileFields(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("listing the company's profile fields: %v", err)
	}
	for _, got := range fields {
		if string(got.Field) == field {
			return got, true
		}
	}
	return crmcontracts.CompanyProfileField{}, false
}

func TestAMeetingFillsHowItsCompanySellsAndCitesTheTranscript(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	e.customerAt(t, company, "Dana Brandt")

	e.read(t, cannedBrain{reply: meetingProfileReply(t, "sales_motion", "They sell through regional distributors.")})

	got, ok := e.profileField(t, company, "sales_motion")
	if !ok || got.Value != "They sell through regional distributors." || got.Source != crmcontracts.CompanyProfileFieldSourceMeeting {
		t.Fatalf("sales_motion reads %+v (present %v), want the meeting's value", got, ok)
	}
	if got.SourceActivityId == nil || ids.UUID(*got.SourceActivityId) != e.activity.UUID || got.SourceOccurredAt == nil {
		t.Errorf("the field cites %v on %v, want the transcript %s and its date", got.SourceActivityId, got.SourceOccurredAt, e.activity.UUID)
	}
	if got.EvidenceSnippet != nil && *got.EvidenceSnippet != "" {
		t.Errorf("the field stores the quote %q; a meeting value cites the meeting instead", *got.EvidenceSnippet)
	}
}

func TestAMeetingNeverReplacesAValueAPersonWrote(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	e.customerAt(t, company, "Dana Brandt")
	if _, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), ids.From[ids.CompanyKind](company), "icp", contacts.ProfileFieldWriteInput{Value: new("Mid-sized banks")}); err != nil {
		t.Fatalf("writing the icp by hand: %v", err)
	}

	e.read(t, cannedBrain{reply: meetingProfileReply(t, "icp", "Insurers")})

	if got, _ := e.profileField(t, company, "icp"); got.Value != "Mid-sized banks" {
		t.Errorf("icp reads %q after the meeting; a person's value stays", got.Value)
	}
}

func TestAMeetingWithTwoCompaniesFillsNeither(t *testing.T) {
	e := setupTranscript(t)
	first, second := e.SeedCompany(t, "Brandt IT", &e.Rep1), e.SeedCompany(t, "Nordkraft", &e.Rep1)
	e.customerAt(t, first, "Dana Brandt")
	e.customerAt(t, second, "Sigrid Berg")

	e.read(t, cannedBrain{reply: meetingProfileReply(t, "sales_motion", "They sell through regional distributors.")})

	for _, company := range []ids.UUID{first, second} {
		if got, ok := e.profileField(t, company, "sales_motion"); ok {
			t.Errorf("a meeting with two companies wrote %q onto one of them", got.Value)
		}
	}
}

func TestAMeetingKeptToItsParticipantsFillsNoField(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	e.customerAt(t, company, "Dana Brandt")
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET audience = 'participants' WHERE id = $1`, e.activity.UUID); err != nil {
		t.Fatal(err)
	}

	e.read(t, cannedBrain{reply: meetingProfileReply(t, "sales_motion", "They sell through regional distributors.")})

	if got, ok := e.profileField(t, company, "sales_motion"); ok {
		t.Errorf("a meeting only its participants may read wrote %q onto a field every reader sees", got.Value)
	}
}
