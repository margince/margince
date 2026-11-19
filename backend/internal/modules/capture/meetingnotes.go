// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"maps"
	"slices"
	"strings"
)

// meetingNotesScopes are the grants that let a connection read the Google Docs
// Meet writes its notes and transcripts into, keyed by provider.
var meetingNotesScopes = map[string][]string{
	providerGdocs: {"https://www.googleapis.com/auth/documents.readonly"},
}

// meetingNotesCalendars are the calendar providers whose meetings a notes
// connection reads, keyed by the notes provider. Notes are read only while
// both connections are on one account.
var meetingNotesCalendars = map[string]string{
	providerGdocs: providerGcal,
}

// MeetingNotesGranted says whether a connection's granted scopes let it read
// meeting notes. A Docs connection whose owner unticked the scope on Google's
// screen holds none, and reads nothing until it is reconnected.
func MeetingNotesGranted(provider string, scopes []string) bool {
	return scopeGranted(meetingNotesScopes, provider, scopes)
}

// MeetingNotesProviders are the providers whose connection reads meeting notes.
func MeetingNotesProviders() []string {
	return slices.Sorted(maps.Keys(meetingNotesScopes))
}

// MeetingNotesConcern is the grant and account policy used before reading notes.
func MeetingNotesConcern(provider string, scopes []string, notes, calendar *string) string {
	if _, paired := meetingNotesCalendars[provider]; !paired {
		return ""
	}
	if !MeetingNotesGranted(provider, scopes) {
		return ConcernNotesGrantMissing
	}
	if notes == nil || calendar == nil {
		return ConcernNotesAccountMismatch
	}
	account := strings.ToLower(bareAddress(*notes))
	if account == "" || account != strings.ToLower(bareAddress(*calendar)) {
		return ConcernNotesAccountMismatch
	}
	return ""
}

const notesCalendarAccountSQL = `(SELECT cal.account_label FROM capture_connection cal
 WHERE cal.user_id = c.user_id AND cal.provider = 'gcal'
 AND cal.status IN ('connected','error') AND cal.archived_at IS NULL
 ORDER BY lower(cal.account_label) = lower(c.account_label) DESC, cal.created_at, cal.id LIMIT 1)`
