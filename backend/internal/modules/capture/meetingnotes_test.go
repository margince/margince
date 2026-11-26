// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/gdocs"
)

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
