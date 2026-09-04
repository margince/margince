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
	// The length guard mirrors organization_description_length (0203): a summary
	// the CHECK would reject skips the write instead of aborting the whole save
	// — the profile-field row still lands, and the column stays writable by a
	// shorter later one. The replace arm tolerates a NULL because clearing the
	// column is one of the things it does; the fill arm refuses one, because
	// filling a column with nothing would answer it forever.
	columnDescription: {
		fill: `UPDATE organization SET description = $2 WHERE id = $1 AND archived_at IS NULL
		       AND description IS NULL AND $2::text IS NOT NULL AND length($2) <= 500`,
		replace: `UPDATE organization SET description = $2 WHERE id = $1 AND archived_at IS NULL
		          AND description IS DISTINCT FROM $2 AND ($2::text IS NULL OR length($2) <= 500)`,
	},
}
