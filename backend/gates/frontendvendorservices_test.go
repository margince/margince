// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Which calendar belongs beside which mailbox is answered on both sides of the
// wire, and the two answers must be the same pairs.
//
// The server chains a mail consent into its vendor's calendar only for the pair
// it knows (also_connect answers 422 otherwise). The screens read the frontend's
// own table to offer Mail and Calendar together, and `oneConsent` to say how
// many screens follow. A side that disagrees promises what the server refuses.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendVendorServices = "../frontend/src/format/connectorprovider.ts"

// tsVendorPair reads one `vendor: { mail: "x", calendar: "y", oneConsent: b }`
// entry, in either quote style, so an entry spelled the other way still counts.
var tsVendorPair = regexp.MustCompile(
	`([a-z]+):\s*\{\s*mail:\s*["']([a-z]+)["'],\s*calendar:\s*["']([a-z]+)["'],\s*oneConsent:\s*(true|false),?\s*\}`)

func TestTheFrontendVendorPairsMatchTheServersChain(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendVendorServices)
	if err != nil {
		t.Fatalf("reading the frontend vendor table: %v", err)
	}
	text := string(source)
	declared := strings.Index(text, "export const VENDOR_SERVICES")
	if declared < 0 {
		t.Fatalf("%s no longer exports VENDOR_SERVICES; this gate is reading a shape that is gone",
			frontendVendorServices)
	}
	body := text[declared:]
	open := indexAfter(body, "= {")
	end := -1
	if open >= 0 {
		end = strings.Index(body[open:], "};")
	}
	if end < 0 {
		t.Fatalf("%s's VENDOR_SERVICES is no longer an object literal this gate can bound",
			frontendVendorServices)
	}
	literal := tsComment.ReplaceAllString(body[open:open+end], " ")

	inTS := map[string]string{}
	oneConsent := map[string]bool{}
	for _, m := range tsVendorPair.FindAllStringSubmatch(literal, -1) {
		inTS[m[2]] = m[3]
		oneConsent[m[2]] = m[4] == "true"
	}
	// Every mail key must have been read: an entry the pattern cannot parse is
	// one this gate would silently agree with.
	if mails := strings.Count(literal, "mail:"); mails != len(inTS) || len(inTS) < 2 {
		t.Fatalf("read %d pairs out of %d mail entries in %s's VENDOR_SERVICES (%v); the read has "+
			"gone short and this gate would pass on a table it cannot see", len(inTS), mails,
			frontendVendorServices, inTS)
	}

	inGo := compose.PairedCalendars()
	for mail, calendar := range inGo {
		if inTS[mail] != calendar {
			t.Errorf("the server chains %s into %s, and %s's VENDOR_SERVICES pairs %s with %q",
				mail, calendar, frontendVendorServices, mail, inTS[mail])
		}
	}
	for mail, calendar := range inTS {
		if oneConsent[mail] != compose.OneConsent(mail) {
			t.Errorf("%s's VENDOR_SERVICES says oneConsent: %t for %s, and compose.OneConsent says %t: "+
				"the screen would promise a different number of consent screens than the vendor shows",
				frontendVendorServices, oneConsent[mail], mail, compose.OneConsent(mail))
		}
		if _, ok := inGo[mail]; !ok {
			t.Errorf("%s's VENDOR_SERVICES pairs %s with %s, and compose.PairedCalendars() chains no "+
				"calendar from %s: the screen would offer a pair the server refuses",
				frontendVendorServices, mail, calendar, mail)
		}
	}
}
