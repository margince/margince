// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package meetingprofile_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/commitmentparty"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/meetingprofile"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const distributors = "They sell through regional distributors."

// meeting is one transcript and the outside contacts in it.
type meeting struct {
	e          *integration.Env
	transcript ids.ActivityID
}

// saidBy is one profile entry a reading reports, said by speaker.
type saidBy struct {
	speaker      ids.UUID
	field, value string
}

func newMeeting(t *testing.T, audience string) *meeting {
	t.Helper()
	e := integration.Setup(t)
	subject, body, sourceSystem := "Rollout call", "Dana: We sell through regional distributors.", "transcript"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, Body: &body, SourceSystem: &sourceSystem, Source: "manual",
		OccurredAt: new(clocktest.Now(t).Add(-time.Hour)),
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &meeting{e: e, transcript: ids.From[ids.ActivityKind](ids.UUID(logged.Id))}
	if audience != "workspace" {
		if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE activity SET audience = $2 WHERE id = $1`, m.transcript.UUID, audience); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

// employee is a contact employed at company.
func (m *meeting) employee(t *testing.T, company ids.UUID, name string) ids.UUID {
	t.Helper()
	contact := m.e.SeedContact(t, name, &m.e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := m.e.Contacts.CreateRelationship(m.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	return contact
}

// read applies a reading that states entries, each said by an outside contact.
func (m *meeting) read(t *testing.T, entries ...saidBy) {
	t.Helper()
	profile := []map[string]any{}
	for _, entry := range entries {
		profile = append(profile, map[string]any{
			"speaker_ref": "contact:" + entry.speaker.String(),
			"field":       entry.field, "value": entry.value, "source_lines": []int{1}, "confidence": 0.8,
		})
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var parties []commitmentparty.Party
	for _, entry := range entries {
		parties = append(parties, commitmentparty.Party{Ref: "contact:" + entry.speaker.String(), Contact: &entry.speaker})
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.WithWorkspaceTx(m.e.Admin(), m.e.Pool, func(tx pgx.Tx) error {
		return meetingprofile.Apply(m.e.Admin(), tx, quiet, raw, 1, m.transcript, parties)
	}); err != nil {
		t.Fatalf("applying the reading: %v", err)
	}
}

func (m *meeting) field(t *testing.T, company ids.UUID, field string) (crmcontracts.CompanyProfileField, bool) {
	t.Helper()
	fields, err := m.e.Contacts.ListCompanyProfileFields(m.e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range fields {
		if string(got.Field) == field {
			return got, true
		}
	}
	return crmcontracts.CompanyProfileField{}, false
}

func TestAMeetingFillsHowItsCompanySellsAndCitesTheTranscript(t *testing.T) {
	m := newMeeting(t, "workspace")
	company := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1)
	m.read(t, saidBy{m.employee(t, company, "Dana Brandt"), "sales_motion", distributors})

	got, ok := m.field(t, company, "sales_motion")
	if !ok || got.Value != distributors || got.Source != crmcontracts.CompanyProfileFieldSourceMeeting {
		t.Fatalf("sales_motion reads %+v (present %v), want the meeting's value", got, ok)
	}
	if got.SourceActivityId == nil || ids.UUID(*got.SourceActivityId) != m.transcript.UUID || got.SourceOccurredAt == nil || got.SourceOccurredPrecision == nil {
		t.Errorf("the field cites %v on %v (%v), want the transcript and its date", got.SourceActivityId, got.SourceOccurredAt, got.SourceOccurredPrecision)
	}
	if got.EvidenceSnippet != nil && *got.EvidenceSnippet != "" {
		t.Errorf("the field stores the quote %q; a meeting value cites the meeting instead", *got.EvidenceSnippet)
	}
}

func TestEachSpeakersValueLandsOnTheirOwnCompany(t *testing.T) {
	m := newMeeting(t, "workspace")
	brandt, nordkraft := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1), m.e.SeedCompany(t, "Nordkraft", &m.e.Rep1)
	dana := m.employee(t, brandt, "Dana Brandt")
	m.employee(t, nordkraft, "Sigrid Berg")
	m.read(t, saidBy{dana, "sales_motion", distributors})
	if got, _ := m.field(t, brandt, "sales_motion"); got.Value != distributors {
		t.Errorf("the speaker's company reads %q, want what they said", got.Value)
	}
	if got, ok := m.field(t, nordkraft, "sales_motion"); ok {
		t.Errorf("the other company got %q, which nobody of theirs said", got.Value)
	}
}

func TestAMeetingNeverWritesOntoOurOwnCompanyOrOverAUser(t *testing.T) {
	m := newMeeting(t, "workspace")
	company := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1)
	dana := m.employee(t, company, "Dana Brandt")
	if _, err := m.e.Contacts.UpdateCompanyProfileField(m.e.Admin(), ids.From[ids.CompanyKind](company), "icp",
		contacts.ProfileFieldWriteInput{Value: new("Mid-sized banks")}); err != nil {
		t.Fatal(err)
	}
	m.read(t, saidBy{dana, "icp", "Insurers"})
	if got, _ := m.field(t, company, "icp"); got.Value != "Mid-sized banks" {
		t.Errorf("icp reads %q; a user's value stays", got.Value)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE company SET is_anchor = true WHERE id = $1`, company); err != nil {
		t.Fatal(err)
	}
	m.read(t, saidBy{dana, "sales_motion", distributors})
	if got, ok := m.field(t, company, "sales_motion"); ok {
		t.Errorf("a meeting wrote %q onto our own company", got.Value)
	}
}

func TestAMeetingKeptToItsParticipantsFillsNoField(t *testing.T) {
	m := newMeeting(t, "participants")
	company := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1)
	m.read(t, saidBy{m.employee(t, company, "Dana Brandt"), "sales_motion", distributors})
	if got, ok := m.field(t, company, "sales_motion"); ok {
		t.Errorf("a meeting only its participants may read wrote %q onto a field every reader sees", got.Value)
	}
}

func TestAMeetingValueGoesWhenTheMeetingNoLongerStatesItOrIsDeleted(t *testing.T) {
	m := newMeeting(t, "workspace")
	company := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1)
	dana := m.employee(t, company, "Dana Brandt")
	m.read(t, saidBy{dana, "sales_motion", distributors}, saidBy{dana, "icp", "Banks"})
	m.read(t, saidBy{dana, "icp", "Banks"})
	if got, ok := m.field(t, company, "sales_motion"); ok {
		t.Errorf("a re-reading that no longer states sales_motion left %q", got.Value)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `DELETE FROM activity WHERE id = $1`, m.transcript.UUID); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.field(t, company, "icp"); ok {
		t.Errorf("the transcript is gone and its value %q stayed", got.Value)
	}
}

func TestAUsersCorrectionStopsCitingTheMeeting(t *testing.T) {
	m := newMeeting(t, "workspace")
	company := m.e.SeedCompany(t, "Brandt IT", &m.e.Rep1)
	m.read(t, saidBy{m.employee(t, company, "Dana Brandt"), "icp", "Banks"})
	got, err := m.e.Contacts.UpdateCompanyProfileField(m.e.Admin(), ids.From[ids.CompanyKind](company), "icp",
		contacts.ProfileFieldWriteInput{Value: new("Regional banks")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != crmcontracts.CompanyProfileFieldSourceHuman || got.SourceActivityId != nil || got.SourceOccurredAt != nil {
		t.Errorf("the corrected field reads source %q citing %v on %v, want a user's value citing no meeting",
			got.Source, got.SourceActivityId, got.SourceOccurredAt)
	}
}
