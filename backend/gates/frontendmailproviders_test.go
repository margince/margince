// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Which capture providers are a MAILBOX is answered on both sides of the wire,
// and the two answers must be the same three names.
//
// The server refuses every mail-shaped operation for a calendar — the four
// backfill ops and the mail posture — and the connections screen draws none of
// those rows against one. A screen that offered what the server refuses is
// exactly how this was reported: a member's Google Calendar row was drawn under
// an envelope, beside their own email address, with a mail-history import card
// that answered "this mailbox type can't be backfilled". They read it as the
// product refusing to import their mail.
//
// Both sides are read here — the Go set from the compose vocabulary that gates
// the refusals, the TypeScript set from the module the screen asks — so neither
// is this gate's own copy, and a provider added to one alone fails.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

const frontendMailProviders = "../frontend/src/screens/connectorproviders.ts"

// tsProviderEntry reads one quoted provider name out of the set literal, in
// either quote style: a name written in the other style than expected would
// parse as NOTHING, and an entry the parser cannot see is one this gate
// silently agrees with.
var tsProviderEntry = regexp.MustCompile(`["']([a-z]+)["']`)

// tsProviderComment strips comments first, so a provider merely NAMED in the
// prose beside the literal cannot stand in for a deleted entry — and the doc
// comment above this literal names all four of them.
var tsProviderComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheFrontendMailProvidersMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendMailProviders)
	if err != nil {
		t.Fatalf("reading the frontend provider split: %v", err)
	}
	const marker = "MAIL_PROVIDERS"
	start := indexAfter(string(source), marker+": ReadonlySet<Provider> = new Set<Provider>([")
	if start < 0 {
		t.Fatalf("%s no longer declares %s as a Set literal — this gate is reading a shape that is gone",
			frontendMailProviders, marker)
	}
	end := indexAfter(string(source)[start:], "]")
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", frontendMailProviders, marker)
	}

	literal := tsProviderComment.ReplaceAllString(string(source)[start:start+end], " ")
	var inTS []string
	for _, m := range tsProviderEntry.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	// Three today. A floor rather than an equality on the count: the set may
	// grow, and what must not happen is this gate reading a SHORTER one than the
	// file carries, which would agree with a server that had lost the same
	// entries.
	if len(inTS) < 3 {
		t.Fatalf("parsed %d providers out of %s (%v) — the set has at least three, so the read has "+
			"gone short and this gate would pass on a half-empty file", len(inTS), frontendMailProviders, inTS)
	}

	inGo := compose.MailProviders()
	if len(inGo) < 3 {
		t.Fatalf("compose.MailProviders() = %v — with fewer than three the server has stopped calling "+
			"a mailbox a mailbox, and the comparison below would hold by agreeing with the loss", inGo)
	}
	sort.Strings(inGo)
	sort.Strings(inTS)
	if strings.Join(inGo, ",") != strings.Join(inTS, ",") {
		t.Errorf("the mailbox/calendar split has drifted.\n"+
			"  %s draws mail rows for: %v\n"+
			"  compose.MailProviders() serves mail ops for: %v\n"+
			"A provider the screen treats as a mailbox and the server does not is a row offering what "+
			"the next call refuses; the reverse is a working capability with no way to reach it.",
			frontendMailProviders, inTS, inGo)
	}
}
