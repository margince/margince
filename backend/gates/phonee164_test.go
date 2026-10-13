// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// contact_phone's E.164 rule is one rule, spelled on both sides of the wire.
//
// Phone normalisation is parsing, not case folding: `+49 30 1234`, `+49301234`,
// `0049301234` and `030 1234` are four spellings of one number, and only the
// E.164 form matches in the dedupe lane. A row that reaches the table
// unnormalised is not untidy — it is invisible to dedupe, and one contact is
// created twice with nothing saying so.
//
// So the Go seam parses and the database refuses, and the two must be the same
// rule rather than two spellings of it. A database LOOSER than the seam admits
// exactly the rows the seam exists to refuse, which relocates the divergence
// instead of closing it; a database STRICTER refuses a number the product has
// already accepted, which is a write that fails after the user was told it
// worked. Both directions are failures, so this fails in both.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// phoneCheck reads the pattern out of contact_phone's CHECK as Postgres prints
// it: `CHECK ((phone ~ '<pattern>'::text))`.
var phoneCheck = regexp.MustCompile(`^public\.contact_phone\.contact_phone_e164 CHECK \(\(phone ~ '(.*)'::text\)\)$`)

func TestTheContactPhoneCheckIsThisPattern(t *testing.T) {
	t.Parallel()
	for _, record := range catalogRecords(t) {
		found := phoneCheck.FindStringSubmatch(strings.TrimSpace(record))
		if found == nil {
			continue
		}
		// Postgres doubles a quote inside a string literal; the Go constant holds
		// the one the regexp engine sees.
		if stored := strings.ReplaceAll(found[1], "''", "'"); stored != values.E164Pattern {
			t.Errorf("contact_phone_e164 holds %q and values.E164Pattern is %q. One rule, two spellings: "+
				"a looser database admits the rows the Go seam refuses — invisible to dedupe, so the same "+
				"contact is created twice — and a stricter one refuses a number the product already "+
				"accepted.", stored, values.E164Pattern)
		}
		return
	}
	t.Errorf("contact_phone carries no contact_phone_e164 CHECK, so the \"E.164 normalized at write\" "+
		"contract is held by %s alone — and the comment on it names one seam create, update and vCard "+
		"import happen to share today.", "values.ParsePhone")
}
