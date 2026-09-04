// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The two marks a company wears, and the one statement shape that moves either.
//
// A company is drawn at two widths and one picture cannot serve both: a
// wordmark scaled into a 32px square is a row of illegible strokes, and a
// square badge stretched across an expanded sidebar head is a logo nobody
// chose. So the record carries a mark per SLOT, each in its own pair of
// columns, each with its own provenance spelling — so a reader of the history
// can tell which of the two a person changed.
//
// Six writers reach those pairs: a person setting the installation's own mark
// and the same person taking it off, twice over, plus a website resolve and a
// site-read confirmation adopting what it found. Each used to carry its own
// copy of this UPDATE. They agreed, which is the state a set of copies is in
// right up until one of them is edited.
//
// CLEARING IS SETTING THE MARK TO NOTHING, so a clear takes this statement too
// with two NULL values rather than a statement of its own. What made a fourth
// copy look necessary was the SET list, and a bind parameter answers that.
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

import "fmt"

// LogoSlot names which of a company's two marks a caller means.
type LogoSlot int

const (
	// LogoWide is the lockup an expanded sidebar and a record page have room
	// for. It is the mark a website read resolves, and the one every
	// organization can wear.
	LogoWide LogoSlot = iota
	// LogoIcon is the square badge a collapsed 56px rail draws. Only the
	// installation's own company wears one today, and only from an upload —
	// no read resolves one, so this slot has no machine writer to hold off.
	LogoIcon
)

// logoFieldName is how the wide mark is spelled in field_provenance and in the
// audit/event delta. One spelling, so the provenance display and the write
// cannot disagree about which field was set.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const logoFieldName = "logo"

// logoIconFieldName is the icon's own spelling, for the same reason and on the
// same terms. It is deliberately NOT the wide mark's: a history that recorded
// both marks under one field name would say a company's logo changed twice and
// leave a reader unable to tell which picture moved.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const logoIconFieldName = "logo_icon"

// logoSlotSpec is where one mark's two values live, how the slot is spelled in
// the record of a change, what its URL looks like, and the statements that read
// and write it — built once at startup rather than per call, so the pool plans
// each statement once.
type logoSlotSpec struct {
	field string
	// urlSuffix is what this slot adds to the wide mark's endpoint path. The
	// wide mark adds nothing, which is what keeps the URL every shipped client
	// already holds exactly as it was.
	urlSuffix string
	write     string
	readKey   string
}

var logoSlotSpecs = [...]logoSlotSpec{
	LogoWide: newLogoSlotSpec(logoFieldName, "", "logo_object_key", "logo_origin"),
	LogoIcon: newLogoSlotSpec(logoIconFieldName, "/icon", "logo_icon_object_key", "logo_icon_origin"),
}

func (s LogoSlot) spec() logoSlotSpec { return logoSlotSpecs[s] }

// newLogoSlotSpec splices a slot's column names into its statements. The names
// are compile-time literals declared on the line above, never a value off a
// request — the one shape of identifier interpolation this tree allows, and
// what lets six writers of a mark read one statement instead of six.
func newLogoSlotSpec(field, urlSuffix, keyColumn, originColumn string) logoSlotSpec {
	return logoSlotSpec{
		field:     field,
		urlSuffix: urlSuffix,
		write: fmt.Sprintf(`UPDATE organization SET %[1]s = $2, %[2]s = $3
	WHERE id = $1 AND archived_at IS NULL
	RETURNING (SELECT o.%[1]s FROM organization o WHERE o.id = $1),
	          (SELECT o.%[2]s FROM organization o WHERE o.id = $1)`, keyColumn, originColumn),
		readKey: fmt.Sprintf(
			`SELECT %s FROM organization WHERE id = $1 AND archived_at IS NULL`, keyColumn),
	}
}
