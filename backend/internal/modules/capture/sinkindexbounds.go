// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The bound between a sender-controlled header and a btree index.
//
// Postgres caps a btree index entry at 8191 bytes and refuses the write above
// it (SQLSTATE 54000). Three columns on `activity` are indexed and filled from
// headers the sender wrote — thread_key, source_id and counterparty_email — so
// whoever sends the mail chooses whether the row can be written at all.
//
// Unbounded, the refusal arrives as a plain driver error from inside the page
// transaction. It matches none of the connector sentinels, so classifySyncError
// calls it `internal`, recordPageFault treats it as a fault no delay repairs,
// and the run ends. One message costs every message behind it: a mailbox that
// stopped this way had imported 300 of 1124 and could not resume, because the
// offending message sits at a fixed position and every retry reaches it again.
//
// So the record is refused HERE, at the sink door, where a refusal is already
// the record's own shape rather than a fault (`admitRecord`), and refused as a
// SKIP: connector.ErrSkip is counted by every sync loop and fails nothing, so
// the page keeps its remaining messages and the import continues.
//
// This is the same hazard mailmap's DSN parsing already guards on its own path,
// where a non-UTF-8 header would fail the write and wedge the mailbox. Same
// reachability — anyone who can send mail to a connected mailbox — and the same
// remedy, applied to size rather than encoding.
//
// The activities module writes the same three columns and does NOT repeat this
// bound, which is deliberate rather than an oversight: every value that arrives
// from outside reaches the row through this sink. Mail and calendar come through
// the connectors, and an extension's inbound webhook builds a NormalizedRecord
// and enters by this same door. What the activities path receives is either
// derived inside the installation or read back off a row this guard already
// admitted, so a second copy of the rule there would bind nothing.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// maxIndexedHeaderChars bounds a header that becomes an indexed identity:
	// the thread key and the natural key's source id. RFC 5322 §2.1.1 caps a
	// header line at 998 octets, so a longer single value is malformed rather
	// than merely large — no legitimate Message-ID or References root reaches
	// it, and the observed maxima in real traffic are under 200.
	maxIndexedHeaderChars = 998

	// maxIndexedAddressChars bounds the counterparty address, which carries two
	// btree indexes of its own. RFC 5321 §4.5.3.1.3 caps a forward path at 320
	// octets; the trace ledger already holds the same number for the same
	// reason.
	maxIndexedAddressChars = 320
)

// admitIndexableKeys refuses a record whose sender-controlled identity values
// are too large for the indexes they land in.
//
// The count is in BYTES, not runes, because the limit Postgres enforces is on
// the stored index entry. A rune count would admit a multi-byte header three
// times over the byte limit and hand the page the very write error this refusal
// exists to prevent.
//
// The message names the field and the bound but never the value: it reaches an
// operator's log, and the value is somebody's mail.
func admitIndexableKeys(rec connector.NormalizedRecord) error {
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{"thread key", rec.ThreadKey, maxIndexedHeaderChars},
		{"source id", rec.NaturalKey.SourceID, maxIndexedHeaderChars},
		{"counterparty address", rec.Counterparty.Email, maxIndexedAddressChars},
	} {
		if len(field.value) > field.limit {
			return fmt.Errorf(
				"capture: the record's %s is %d bytes, over the %d-byte bound its index can hold: %w",
				field.name, len(field.value), field.limit, connector.ErrSkip)
		}
	}
	return nil
}
