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
// And the half in front of the parameter: every SCREEN a mailed link addresses
// is one the router still answers to. The two read different frontend files
// because they are different lists — a Brief view is a parameter on one screen,
// and the notice mail's link names another screen entirely.
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
	frontendRouter    = "../frontend/src/app/router.tsx"
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

// The SCREEN half of the same pair, and it is a different frontend file.
//
// A Brief view is a parameter on one screen; the address BEFORE that parameter
// is a screen in the router's own SCREENS list, and the notice mail's link
// names one that is not the Brief at all (the Worklist). The silent-drift
// argument is identical — `parseRoute` answers "not-found" for an address this
// app does not have, and the shell renders that rather than erroring — so a
// screen renamed on one side leaves every reader of the message somewhere that
// says nothing about what they were told was waiting.
//
// Derived from BOTH sides: every *Fragment constant the catalog spells, against
// the list the router exports. A fragment added later is covered without
// editing this gate.
func TestEveryMailedLinkNamesAScreenTheAppStillAnswersTo(t *testing.T) {
	t.Parallel()
	back := readFiscalSource(t, backendMailLink)
	offered := screensOffered(t, readFiscalSource(t, frontendRouter))

	// Every fragment constant, then the screen each one addresses. Counted
	// apart so a fragment written in a shape the address pattern does not match
	// fails here rather than being silently left out of the check.
	declared := regexp.MustCompile(`\w+Fragment\s*=`).FindAllString(back, -1)
	if len(declared) == 0 {
		t.Fatal("no *Fragment constants in " + backendMailLink + " — this gate can no longer see its subject")
	}
	addressed := regexp.MustCompile(`(\w+Fragment)\s*=\s*"/#/([a-z-]+)`).FindAllStringSubmatch(back, -1)
	if len(addressed) != len(declared) {
		t.Fatalf("%s declares %d fragment(s) and %d of them spell an address this gate can read: "+
			"a fragment it cannot see is one no check holds", backendMailLink, len(declared), len(addressed))
	}

	for _, fragment := range addressed {
		if !contains(offered, fragment[2]) {
			t.Errorf(
				"%s addresses screen %q, which %s no longer offers (%v). The app renders "+
					"not-found for an address it does not have, so every reader of that message "+
					"lands nowhere and nothing fails.",
				fragment[1], fragment[2], frontendRouter, offered)
		}
	}
}

// screensOffered reads the router's own SCREENS list rather than restating it.
// A gate that hard-codes part of its subject has become a second copy of it.
func screensOffered(t *testing.T, source string) []string {
	t.Helper()
	list := regexp.MustCompile(`SCREENS\s*=\s*\[([^\]]*)\]`).FindStringSubmatch(source)
	if list == nil {
		t.Fatal("no SCREENS list in " + frontendRouter + " — this gate can no longer see its subject")
	}
	screens := regexp.MustCompile(`"([a-z-]+)"`).FindAllStringSubmatch(list[1], -1)
	if len(screens) == 0 {
		t.Fatalf("the SCREENS list in %s named no screens: %q", frontendRouter, list[1])
	}
	out := make([]string, 0, len(screens))
	for _, screen := range screens {
		out = append(out, screen[1])
	}
	return out
}
