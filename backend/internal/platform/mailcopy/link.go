// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"net/url"
	"strings"
)

// The Brief's two addresses, as a message must spell them.
//
// The morning is the view the Brief opens on, so it needs no parameter: its
// address is the page itself. frontend/src/features/brief/brief.view.ts writes
// only what differs from that default. The weekly names itself.
//
// Held by backend/gates/mailbrieflink_test.go, which reads the view words out
// of the frontend's own VIEWS list: a rename there without one here would leave
// these links pointing at a view the app no longer answers to, and the app
// falls back to the morning silently rather than erroring.
const (
	BriefMorningFragment = "/#/home"
	BriefWeeklyFragment  = "/#/home?view=weekly"
)

// WorklistFragment is where a decision waiting on a colleague is answered.
//
// A SCREEN of its own rather than a view of the Brief, which is why it is not
// spelled Brief*: the Worklist is a top-level address in the frontend's own
// SCREENS list, and the two mailed-link gates read different halves of the
// frontend for that reason.
//
// Held by backend/gates/mailbrieflink_test.go's screen arm, which reads the
// router's SCREENS list: the app resolves an unknown screen by falling back to
// Home, so a rename there without one here would land every reader on a page
// that says nothing about the decision they were told was waiting, and nothing
// on either side would fail.
const WorklistFragment = "/#/worklist"

// These destinations are shared by product mail and checked against the router.
const (
	PreferencesFragment   = "/#/settings/notifications"
	DealRecordFragment    = "/#/deals/"
	ContactRecordFragment = "/#/contacts/"
	CompanyRecordFragment = "/#/companies/"
	LeadRecordFragment    = "/#/leads/"
	ProjectRecordFragment = "/#/projects/"
)

// RecordFragment is shared by item and digest links to authorized record pages.
func RecordFragment(kind, id string) string {
	switch kind {
	case "approval":
		return BriefMorningFragment + "?approval=" + id
	case "deal":
		return DealRecordFragment + id
	case "contact":
		return ContactRecordFragment + id
	case "company":
		return CompanyRecordFragment + id
	case "lead":
		return LeadRecordFragment + id
	case "project":
		return ProjectRecordFragment + id
	}
	return ""
}

// Destination builds the same configured-origin route for text and semantic actions.
func Destination(base, fragment string) string {
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + fragment
}

// TeamFragment retains the selected team and reporting week when opening the app.
func TeamFragment(team, week string) string {
	params := url.Values{"scope": {"team"}, "team": {team}}
	if week != "" {
		params.Set("view", "weekly")
		params.Set("week", week)
	}
	return BriefMorningFragment + "?" + params.Encode()
}

// WeeklyFragment keeps an older report linked to its own frozen week.
func WeeklyFragment(week string) string {
	return BriefWeeklyFragment + "&week=" + url.QueryEscape(week)
}
