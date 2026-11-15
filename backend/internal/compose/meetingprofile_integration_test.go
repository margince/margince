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

// saidBy is one profile entry a reading reports, said by speaker.
type saidBy struct {
	speaker      ids.UUID
	field, value string
}

// meetingProfileReply is a reading that states no next step and the given
// profile entries, each cited to line 1.
func meetingProfileReply(t *testing.T, entries ...saidBy) string {
	t.Helper()
	profile := []map[string]any{}
	for _, entry := range entries {
		profile = append(profile, map[string]any{
			"speaker_ref": "contact:" + entry.speaker.String(), "field": entry.field, "value": entry.value,
			"source_lines": []int{1}, "confidence": 0.8,
		})
	}
	raw, err := json.Marshal(map[string]any{"proposals": []any{}, "company_profile": profile})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// customerAt links a new employee of company to the transcript.
func (e *transcriptEnv) customerAt(t *testing.T, company ids.UUID, name string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	e.linkTranscriptContact(t, contact)
	return contact
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

const distributors = "They sell through regional distributors."

func TestAMeetingFillsHowItsCompanySellsAndCitesTheTranscript(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	dana := e.customerAt(t, company, "Dana Brandt")

	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "sales_motion", distributors})})

	got, ok := e.profileField(t, company, "sales_motion")
	if !ok || got.Value != distributors || got.Source != crmcontracts.CompanyProfileFieldSourceMeeting {
		t.Fatalf("sales_motion reads %+v (present %v), want the meeting's value", got, ok)
	}
	if got.SourceActivityId == nil || ids.UUID(*got.SourceActivityId) != e.activity.UUID || got.SourceOccurredAt == nil {
		t.Errorf("the field cites %v on %v, want the transcript %s and its date", got.SourceActivityId, got.SourceOccurredAt, e.activity.UUID)
	}
	if got.EvidenceSnippet != nil && *got.EvidenceSnippet != "" {
		t.Errorf("the field stores the quote %q; a meeting value cites the meeting instead", *got.EvidenceSnippet)
	}
}

func TestEachSpeakersValueLandsOnTheirOwnCompany(t *testing.T) {
	e := setupTranscript(t)
	brandt, nordkraft := e.SeedCompany(t, "Brandt IT", &e.Rep1), e.SeedCompany(t, "Nordkraft", &e.Rep1)
	dana := e.customerAt(t, brandt, "Dana Brandt")
	e.customerAt(t, nordkraft, "Sigrid Berg")

	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "sales_motion", distributors})})

	if got, _ := e.profileField(t, brandt, "sales_motion"); got.Value != distributors {
		t.Errorf("the speaker's company reads %q, want what they said", got.Value)
	}
	if got, ok := e.profileField(t, nordkraft, "sales_motion"); ok {
		t.Errorf("the other company in the meeting got %q, which its people never said", got.Value)
	}
}

func TestAMeetingNeverWritesOntoOurOwnCompanyOrOverAPerson(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	dana := e.customerAt(t, company, "Dana Brandt")
	if _, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), ids.From[ids.CompanyKind](company), "icp",
		contacts.ProfileFieldWriteInput{Value: new("Mid-sized banks")}); err != nil {
		t.Fatalf("writing the icp by hand: %v", err)
	}
	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "icp", "Insurers"})})
	if got, _ := e.profileField(t, company, "icp"); got.Value != "Mid-sized banks" {
		t.Errorf("icp reads %q after the meeting; a person's value stays", got.Value)
	}

	if _, err := e.owner.Exec(context.Background(), `UPDATE company SET is_anchor = true WHERE id = $1`, company); err != nil {
		t.Fatal(err)
	}
	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "sales_motion", distributors})})
	if got, ok := e.profileField(t, company, "sales_motion"); ok {
		t.Errorf("a meeting wrote %q onto our own company", got.Value)
	}
}

func TestAMeetingKeptToItsParticipantsFillsNoField(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	dana := e.customerAt(t, company, "Dana Brandt")
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET audience = 'participants' WHERE id = $1`, e.activity.UUID); err != nil {
		t.Fatal(err)
	}

	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "sales_motion", distributors})})

	if got, ok := e.profileField(t, company, "sales_motion"); ok {
		t.Errorf("a meeting only its participants may read wrote %q onto a field every reader sees", got.Value)
	}
}

func TestAMeetingValueGoesWhenTheMeetingNoLongerStatesItOrIsDeleted(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	dana := e.customerAt(t, company, "Dana Brandt")
	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "sales_motion", distributors}, saidBy{dana, "icp", "Banks"})})

	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "icp", "Banks"})})
	if got, ok := e.profileField(t, company, "sales_motion"); ok {
		t.Errorf("a re-reading that no longer states sales_motion left %q", got.Value)
	}

	if _, err := e.owner.Exec(context.Background(), `DELETE FROM activity WHERE id = $1`, e.activity.UUID); err != nil {
		t.Fatalf("deleting the transcript: %v", err)
	}
	if got, ok := e.profileField(t, company, "icp"); ok {
		t.Errorf("the transcript is gone and its value %q stayed", got.Value)
	}
}

func TestAPersonsCorrectionStopsCitingTheMeeting(t *testing.T) {
	e := setupTranscript(t)
	company := e.SeedCompany(t, "Brandt IT", &e.Rep1)
	dana := e.customerAt(t, company, "Dana Brandt")
	e.read(t, cannedBrain{reply: meetingProfileReply(t, saidBy{dana, "icp", "Banks"})})

	got, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), ids.From[ids.CompanyKind](company), "icp",
		contacts.ProfileFieldWriteInput{Value: new("Regional banks")})
	if err != nil {
		t.Fatalf("correcting the icp: %v", err)
	}
	if got.Source != crmcontracts.CompanyProfileFieldSourceHuman || got.SourceActivityId != nil || got.SourceOccurredAt != nil {
		t.Errorf("the corrected field reads source %q citing %v on %v, want a person's value citing no meeting",
			got.Source, got.SourceActivityId, got.SourceOccurredAt)
	}
}
