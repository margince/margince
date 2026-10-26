// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A batch of records handed over with the position a restart carries on from.
//
//margince:extension-surface

package extension

import (
	"fmt"
	"unicode/utf8"
)

// MaxBatchRecords caps one IngestBatch. It matches the page size of the file
// importer, so a batch costs the core what one importer page costs.
const MaxBatchRecords = 200

// MaxPositionBytes caps the position a unit saves. A position is the unit's own
// cursor, such as a page token or a last-seen id, and the core never reads it.
const MaxPositionBytes = 512

// SourceName names one of the calling unit's declared ingress sources on the
// Runtime, as OAuthProvider names an OAuth app. It resolves against the unit the
// Runtime was minted for, so it cannot name another unit's source.
type SourceName string

// BatchPosition is the unit's own cursor text, saved with a batch. A distinct
// type so the Runtime has no bare string parameter.
type BatchPosition string

// ValidateBatch holds the shape of one IngestBatch call. Every rule is about the
// call alone, so the core and a unit's own tests can both run it.
func ValidateBatch(system SourceName, recs []Record, position BatchPosition) error {
	if position == "" || len(position) > MaxPositionBytes || !utf8.ValidString(string(position)) {
		return fmt.Errorf("%w: a batch saves a position of 1 to %d bytes of text", ErrInvalid, MaxPositionBytes)
	}
	if len(recs) > MaxBatchRecords {
		return fmt.Errorf("%w: a batch holds at most %d records, got %d", ErrInvalid, MaxBatchRecords, len(recs))
	}
	for i, rec := range recs {
		if rec.System != string(system) {
			return fmt.Errorf("%w: record %d is from %q, but the batch is for %q", ErrInvalid, i, rec.System, system)
		}
	}
	return nil
}
