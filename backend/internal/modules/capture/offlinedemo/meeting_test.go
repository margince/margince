// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package offlinedemo

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestGeneratedMeetingsFileThroughTheirAttendees(t *testing.T) {
	box := demoMailbox()
	meetings := 0
	for _, message := range generate(box, box.Accounts[0]) {
		if message.Kind != "meeting" {
			continue
		}
		meetings++
		record := message.record()
		for _, link := range record.Links {
			if link.Type == datasource.EntityOrganization {
				t.Fatal("meeting links a company directly")
			}
		}
		found := false
		for _, party := range record.Participants {
			if party.Email == box.Accounts[0].People[0].Email && party.Role == connector.ParticipantRoleAttendee {
				found = true
			}
		}
		if !found {
			t.Fatalf("meeting lost its attendee: %+v", record.Participants)
		}
	}
	if meetings == 0 {
		t.Fatal("fixture generated no meetings")
	}
}
