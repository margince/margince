// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import "strings"

// Link is the closing line every message ends with: a label, and the address it
// names on its own indented line.
//
// ONE writer for both senders. The morning and the weekly each had their own
// copy of the same four-part shape — blank line, label, two spaces, address —
// differing only in which label they reached for, which is two answers to one
// question and the thing this package exists to prevent.
//
// A blank base omits the whole line rather than writing a label over nothing.
// An installation that has not been told its own public origin cannot produce a
// working link, and a label followed by an empty indent reads as a link that
// failed to render.
//
// FRAGMENT, NOT PATH. The app is a hash-routed single page, so the address of a
// view lives after the '#'. Both mails linked to the bare origin before this,
// which meant a rep opening the WEEKLY message landed on the morning — the
// message is about a week the page it opened was not showing.
func Link(b *strings.Builder, base, fragment, label string) {
	if base == "" {
		return
	}
	b.WriteString("\n" + label + "\n  " + strings.TrimSuffix(base, "/") + fragment + "\n")
}

// The Brief's two addresses, as a message must spell them.
//
// The morning is the view the Brief opens on, so it needs no parameter — its
// address is the page itself, and frontend/src/screens/brief.view.ts writes
// only what DIFFERS from that default. The weekly names itself.
//
// Held by backend/gates/mailbrieflink_test.go, which reads the view words out
// of the frontend's own VIEWS list: a rename there without one here would leave
// these links pointing at a view the app no longer answers to, and the app
// falls back to the morning silently rather than erroring.
const (
	BriefMorningFragment = "/#/home"
	BriefWeeklyFragment  = "/#/home?view=weekly"
)
