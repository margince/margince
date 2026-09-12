// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two marks a company wears, and the one statement that moves either.
//
// A company is drawn at two widths and one picture cannot serve both: a
// wordmark scaled into a 32px square is a row of illegible strokes, and a
// square badge stretched across an expanded sidebar head is a logo nobody
// chose. So the record carries a mark per SLOT, each in its own pair of
// columns, each with its own provenance spelling — so a reader of the history
// can tell which of the two a contact changed.
//
// WHICH SLOT IS A PARAMETER, NOT A SECOND STATEMENT. Six writers reach these
// four columns: a contact setting the installation's wordmark and the same
// contact taking it off, the same pair again for the badge, a website resolve,
// and a site-read confirmation adopting what it found — in both slots, for the
// cold start that resolved a badge beside the lockup. Every one of them used
// to be a candidate for its own copy of this UPDATE, and copies agree right up
// until one of them is edited. The CASE arms are what a bind parameter costs
// here, and they buy a statement that no writer can spell differently.
//
// They also keep the statement WHERE ITS CALLERS ARE. Splicing the column names
// into a template inside a builder read better and cost more than it looked:
// `writeauthorityreach_test.go` and `updateguard_test.go` both find a mutation
// by reading the SQL a function reaches, so a statement assembled somewhere else
// takes all six writers out of two security gates at once and leaves them
// passing.
//
// CLEARING IS SETTING THE MARK TO NOTHING, so a clear takes this statement too
// with two NULL values rather than a statement of its own.
//
// The RETURNING is the load-bearing half and the reason no caller may write its
// own: the pre-write key comes back from the statement that superseded it, so
// the caller can collect the object nothing references any more. Read separately
// afterwards it would name whatever the NEXT write had since put there, and the
// bytes still in use would be the ones collected.
//
// `archived_at IS NULL` is here rather than at each call site for the same
// reason: a row visible when the write was authorized can be archived before the
// write lands, and pgx.ErrNoRows is how every caller learns that happened.
//
// The casts are not decoration: a clear binds NULL for $2 and $3, and without
// them Postgres has no side of the CASE from which to infer a type.
const companyLogoWrite = `UPDATE company SET
		logo_object_key      = CASE WHEN $4::boolean THEN $2::text ELSE logo_object_key END,
		logo_origin          = CASE WHEN $4::boolean THEN $3::text ELSE logo_origin END,
		logo_icon_object_key = CASE WHEN $4::boolean THEN logo_icon_object_key ELSE $2::text END,
		logo_icon_origin     = CASE WHEN $4::boolean THEN logo_icon_origin ELSE $3::text END
	WHERE id = $1 AND archived_at IS NULL
	RETURNING (SELECT CASE WHEN $4::boolean THEN o.logo_object_key ELSE o.logo_icon_object_key END
	             FROM company o WHERE o.id = $1),
	          (SELECT CASE WHEN $4::boolean THEN o.logo_origin ELSE o.logo_icon_origin END
	             FROM company o WHERE o.id = $1)`

// companyLogoKeyRead answers where one slot's bytes live, taking the slot the same
// way the write does. Live rows only, matching the write: an archived record
// has no mark to serve.
const companyLogoKeyRead = `SELECT CASE WHEN $2::boolean THEN logo_object_key ELSE logo_icon_object_key END
	FROM company WHERE id = $1 AND archived_at IS NULL`

// LogoSlot names which of a company's two marks a caller means.
type LogoSlot int

const (
	// LogoWide is the lockup an expanded sidebar and a record page have room
	// for. It is the mark every website read resolves, and the one every
	// company can wear.
	LogoWide LogoSlot = iota
	// LogoIcon is the square badge a collapsed 56px rail draws. Only the
	// installation's own company wears one today: the cold-start read resolves
	// it from the site's icons beside the lockup (compose/sitelockup.go), and
	// a contact's upload replaces it — so this slot obeys the same
	// human-precedence rule the wide one does.
	LogoIcon
)

// logoFieldName is how the wide mark is spelled in field_provenance and in the
// audit/event delta. One spelling, so the provenance display and the write
// cannot disagree about which field was set.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const logoFieldName = "logo"

// logoIconFieldName is the badge's own spelling, for the same reason and on the
// same terms. It is deliberately NOT the wide mark's: a history that recorded
// both marks under one field name would say a company's logo changed twice and
// leave a reader unable to tell which picture moved.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const logoIconFieldName = "logo_icon"

// String is how the slot is named to a contact — in a log line, an error — and
// it is the field's own spelling, so the name a reader sees is the one they can
// grep for in the record of a change.
func (s LogoSlot) String() string { return s.field() }

// wide is the slot as the statements above take it — a named method rather than
// a `slot == LogoWide` comparison at each call site, because that comparison IS
// the meaning of the bind parameter and a call site that inverted it would
// quietly write the other picture.
func (s LogoSlot) wide() bool { return s == LogoWide }

// field is how this slot is named in the record of a change.
func (s LogoSlot) field() string {
	if s == LogoIcon {
		return logoIconFieldName
	}
	return logoFieldName
}

// urlSuffix is what this slot adds to the wide mark's endpoint path. The wide
// mark adds nothing, which is what keeps the URL every shipped client already
// holds exactly as it was.
func (s LogoSlot) urlSuffix() string {
	if s == LogoIcon {
		return "/icon"
	}
	return ""
}
