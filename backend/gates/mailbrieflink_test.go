// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The Brief's address is spelled twice: the frontend routes it
// (frontend/src/screens/brief.view.ts) and outbound mail links to it
// (internal/platform/mailcopy/link.go), because a message has to name a view
// before the app it opens is running.
//
// This pair drifts SILENTLY, and in the one direction nobody notices. The app
// is hash-routed and resolves an unknown view by FALLING BACK to the morning —
// brief.view.ts's `viewFrom` ends `?? DEFAULT_ADDRESS.view`, on purpose, so a
// stale bookmark opens a working page instead of an error. A mail carrying a
// renamed view therefore keeps working: every reader lands on the morning, the
// weekly message opens today's queue, and no test on either side fails. That is
// exactly the defect this gate exists for — it is the one the weekly mail
// shipped with, linking to the bare origin and landing a Monday summary of last
// week on this morning's work.
//
// WHAT IT HOLDS: every view word the mail fragments name is a view the frontend
// still offers, and the morning — the Brief's default — carries no parameter,
// because the frontend's own writer emits only what DIFFERS from the default.
//
// WHAT IT CANNOT SEE: whether the app's route table still answers to "home", or
// whether the parameter is still called "view". Both are read off the frontend
// as literals here rather than derived, so a rename of either passes this and
// is caught by frontend/src/screens/brief.view.test.ts round-tripping every
// combination it offers.

import (
	"regexp"
	"strings"
	"testing"
)

const (
	frontendBriefView = "../frontend/src/screens/brief.view.ts"
	backendMailLink   = "internal/platform/mailcopy/link.go"
)

// viewsOffered reads the frontend's own VIEWS list rather than restating it.
// A gate that hard-codes part of its subject has become a second copy of it.
func viewsOffered(t *testing.T, source string) []string {
	t.Helper()
	list := regexp.MustCompile(`VIEWS\s*=\s*\[([^\]]*)\]`).FindStringSubmatch(source)
	if list == nil {
		t.Fatal("no VIEWS list in " + frontendBriefView + " — this gate can no longer see its subject")
	}
	views := regexp.MustCompile(`"([a-z_]+)"`).FindAllStringSubmatch(list[1], -1)
	if len(views) == 0 {
		t.Fatalf("the VIEWS list in %s named no views: %q", frontendBriefView, list[1])
	}
	out := make([]string, 0, len(views))
	for _, view := range views {
		out = append(out, view[1])
	}
	return out
}

func TestEveryMailedBriefLinkNamesAViewTheAppStillOffers(t *testing.T) {
	t.Parallel()
	front := readFiscalSource(t, frontendBriefView)
	back := readFiscalSource(t, backendMailLink)

	offered := viewsOffered(t, front)
	// The fragments as the mail actually spells them, read out of the source so
	// a fragment added later is covered without editing this gate.
	fragments := regexp.MustCompile(`Brief\w+Fragment\s*=\s*"([^"]*)"`).FindAllStringSubmatch(back, -1)
	if len(fragments) == 0 {
		t.Fatal("no Brief*Fragment constants in " + backendMailLink + " — this gate can no longer see its subject")
	}

	named := regexp.MustCompile(`view=([a-z_]+)`)
	for _, fragment := range fragments {
		address := fragment[1]
		asked := named.FindStringSubmatch(address)
		if asked == nil {
			// No parameter means the DEFAULT view, which is what the frontend's
			// writer emits for it. Nothing to check against the list.
			continue
		}
		if !contains(offered, asked[1]) {
			t.Errorf(
				"a mailed link names view %q, which %s no longer offers (%v). "+
					"The app falls back to the morning silently, so every reader "+
					"of that message lands on the wrong page and nothing fails.",
				asked[1], frontendBriefView, offered)
		}
	}
}

// The morning is the Brief's default view, so its link carries no parameter.
//
// Not cosmetic: brief.view.ts's `paramsFor` writes only what DIFFERS from
// DEFAULT_ADDRESS, so a mail naming the default would be a second spelling of
// an address the app itself never produces — and the day the default moves, the
// mail would keep asking for a view the product no longer opens on.
func TestTheMorningsMailedLinkAsksForNoView(t *testing.T) {
	t.Parallel()
	back := readFiscalSource(t, backendMailLink)

	morning := regexp.MustCompile(`BriefMorningFragment\s*=\s*"([^"]*)"`).FindStringSubmatch(back)
	if morning == nil {
		t.Fatal("no BriefMorningFragment in " + backendMailLink)
	}
	if strings.Contains(morning[1], "view=") {
		t.Errorf(
			"the morning's mailed link is %q, which names a view. The morning is "+
				"the default, and the frontend writes only what differs from it.",
			morning[1])
	}
}
