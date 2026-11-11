// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/gdocs"
)

func TestMeetingNotesGrantMatchesFrontend(t *testing.T) {
	grants := frontendScopeTable(t, "MEETING_NOTES_SCOPES")
	if len(meetingNotesScopes) == 0 || len(grants) != len(meetingNotesScopes) {
		t.Fatalf("frontend providers %v do not match backend %v", grants, meetingNotesScopes)
	}
	for provider, scopes := range meetingNotesScopes {
		if len(scopes) == 0 || !slices.Equal(scopes, grants[provider]) {
			t.Errorf("%s: backend scopes %v, frontend scopes %v", provider, scopes, grants[provider])
		}
	}
}

func TestMeetingNotesGrantReadsTheDocsConsentScope(t *testing.T) {
	if !MeetingNotesGranted("gdocs", gdocs.ConsentScopes()) {
		t.Error("a full Docs consent does not grant meeting notes")
	}
	if MeetingNotesGranted("gdocs", []string{gdocs.EmailScope}) {
		t.Error("a Docs consent without documents.readonly grants meeting notes")
	}
	if MeetingNotesGranted("gcal", []string{gdocs.ReadScope}) {
		t.Error("a calendar connection grants meeting notes")
	}
	if !slices.Equal(MeetingNotesProviders(), []string{"gdocs"}) {
		t.Errorf("meeting-notes providers %v, want [gdocs]", MeetingNotesProviders())
	}
}
