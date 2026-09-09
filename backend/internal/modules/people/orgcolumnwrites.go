// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The statements that write a company's editable columns, in ONE table.
//
// Three writers reach these four columns — the human company form, the
// read-back's fill arm, and the read-back's overwrite arm — and each used to
// carry its own spelling of the same UPDATE. Twelve statements said eight
// things, and the drift between them was not theoretical: the description
// length guard was written three different ways, four statements hand-wrote an
// `updated_at = now()` the table's own trigger already sets, and the two
// read-back arms looked like they differed on it when they did not.
//
// The second dimension is the only one that honestly separates the three: not
// which column, but whether this writer may put its value onto a column
// somebody has already answered. A form typing a legal name replaces; a
// read-back filling an empty one does not get to.
//
// EACH WRITER STILL NAMES THIS TABLE ITSELF, and that is deliberate rather than
// clumsy. The censuses that hold the organization writers together — the rename
// re-check's, the write-authority probe's, the concurrency guard's — attribute a
// statement to the function that NAMES it, so three writers calling one shared
// resolver would collapse into one resolver naming one table, and three
// obligations would become one the resolver could not discharge. Sharing the
// text is the whole win here; routing the send through a helper would trade
// three duplicate statements for three weaker gates.
//
// Every statement carries `archived_at IS NULL`. The probes at the three entry
// points already refuse an archived company, and this is the same rule where it
// cannot be skipped: between a probe and its write there is a window, and the
// column set below is the most contended in the product.

// orgWriteAuthority says what a writer may do to a column somebody has already
// answered. It has no zero value on purpose — a writer states which it holds.
import "strings"

type orgWriteAuthority uint8

const (
	// fillUnclaimed writes only where the column is still unanswered.
	fillUnclaimed orgWriteAuthority = iota + 1
	// replaceStanding writes over a value already there.
	replaceStanding
)

// orgColumnWrite is one column's pair of statements, keyed below by the
// column name the read-back and the form both use.
type orgColumnWrite struct {
	fill    string
	replace string
}

// statementFor returns the statement this authority sends, and whether the
// column is one this table writes at all.
func (w orgColumnWrite) statementFor(authority orgWriteAuthority) string {
	if authority == fillUnclaimed {
		return w.fill
	}
	return w.replace
}

// orgColumnWrites is the one place an organization's editable columns are
// written. The identifier is never a bind parameter: the statement is fixed
// here and only values bind.
var orgColumnWrites = map[string]orgColumnWrite{
	columnLegalName: {
		fill:    `UPDATE organization SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL AND legal_name IS NULL`,
		replace: `UPDATE organization SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL AND legal_name IS DISTINCT FROM $2`,
	},
	columnIndustry: {
		fill:    `UPDATE organization SET industry = $2 WHERE id = $1 AND archived_at IS NULL AND industry IS NULL`,
		replace: `UPDATE organization SET industry = $2 WHERE id = $1 AND archived_at IS NULL AND industry IS DISTINCT FROM $2`,
	},
	// A scraped registered address arrives as one formatted line, so it fills
	// line1 only while no STRUCTURED address exists — otherwise it would write a
	// second, worse spelling of an address the record already holds properly.
	//
	// Nothing here about geocoding: a trigger marks the coordinates stale on any
	// address column that changes (the organization_geocode migration), so no
	// address writer can forget it. An earlier version did it in the statement —
	// correct, and something the next address writer would not have copied.
	columnAddress: {
		fill: `UPDATE organization SET address_line1 = $2 WHERE id = $1 AND archived_at IS NULL
		       AND address_line1 IS NULL AND address_city IS NULL AND address_postal_code IS NULL`,
		replace: `UPDATE organization SET address_line1 = $2 WHERE id = $1 AND archived_at IS NULL
		          AND address_line1 IS DISTINCT FROM $2`,
	},
	// No length guard: the value arrives already bounded by headerLine, so a
	// statement that skipped an overlong one could only skip a value nothing
	// produces. The guard used to be here and it SILENTLY dropped the write —
	// a 501-character summary the contract accepts left the header blank with
	// nothing said to the caller, no audit entry, and no way to find out why.
	//
	// The replace arm tolerates a NULL because clearing the column is one of the
	// things it does; the fill arm refuses one, because filling a column with
	// nothing would answer it forever.
	columnDescription: {
		fill: `UPDATE organization SET description = $2 WHERE id = $1 AND archived_at IS NULL
		       AND description IS NULL AND $2::text IS NOT NULL`,
		replace: `UPDATE organization SET description = $2 WHERE id = $1 AND archived_at IS NULL
		          AND description IS DISTINCT FROM $2`,
	},
}

// orgDescriptionMax is what organization_description_length admits — the CHECK
// bounding the header line the company view renders. Held by the schema itself:
// a write over this length is refused by the column, which is why the value is
// bounded before it gets there rather than tested once it arrives.

const orgDescriptionMax = 500

// headerLine renders a profile value as the display line the description
// column can hold.
//
// The bound is baked in rather than taken as a parameter, because description
// is the only capped column among the four this file writes — a parameter would
// take one value at every call site and read as though it took several.
//
// WHY A PREFIX AND NOT A GUARD. `offer_summary` is what an installation says
// about what it sells, and the contract accepts far more of it than the column
// holds. The header is ONE RENDERING of that — the column's own comment frames
// description as a display line — so a value too long for the line is a value
// to shorten, not a write to drop. Dropping it is what happened before: the summary saved, the
// header stayed blank, and nothing anywhere said why.
//
// Capping the source to fit the rendering was the other option and it is the
// wrong direction: it shortens what a company may say about itself so that one
// of its displays fits.
//
// CUT AT A WORD BOUNDARY, and add nothing. An ellipsis would be this function
// inventing punctuation into a field a human wrote, and the header is a line
// the reader sees rather than a string another system parses — a word cut in
// half reads as a bug in the product, where a clean stop reads as a summary.
// If there is no boundary to cut at (one very long word), the hard cut stands:
// a bounded value beats an empty one.
//
// Runes, not bytes: the CHECK counts characters (Postgres length() does), so a
// byte cut would both refuse text the column accepts and split a multi-byte
// character.
func headerLine(value string) string {
	runes := []rune(value)
	if len(runes) <= orgDescriptionMax {
		return value
	}
	cut := string(runes[:orgDescriptionMax])
	if at := strings.LastIndexAny(cut, " \t\n"); at > 0 {
		return strings.TrimRight(cut[:at], " \t\n")
	}
	return cut
}
