// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"maps"
	"slices"
)

// meetingNotesScopes are the grants that let a connection read the Google Docs
// Meet writes its notes and transcripts into, keyed by provider. Held equal to
// frontend/src/lib/connector-status.ts's MEETING_NOTES_SCOPES by a parity test.
var meetingNotesScopes = map[string][]string{
	"gdocs": {"https://www.googleapis.com/auth/documents.readonly"},
}

// MeetingNotesGranted says whether a connection's granted scopes let it read
// meeting notes. A Docs connection whose owner unticked the scope on Google's
// screen holds none, and reads nothing until it is reconnected.
func MeetingNotesGranted(provider string, scopes []string) bool {
	for _, grant := range meetingNotesScopes[provider] {
		if slices.Contains(scopes, grant) {
			return true
		}
	}
	return false
}

// MeetingNotesProviders are the providers whose connection reads meeting notes.
func MeetingNotesProviders() []string {
	return slices.Sorted(maps.Keys(meetingNotesScopes))
}
