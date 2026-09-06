// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The bound has to be enforced where the sink admits records, not only in
// isolation. Checked any later and the oversized value is already inside the
// page transaction, where the write error ends the run before any skip can be
// counted — which is the whole failure.
//
// The nil pool is the assertion: reaching the database at all would panic, so a
// clean skip proves the refusal happened at the door.
func TestTheSinkSkipsAnUnindexableRecordBeforeTouchingThePool(t *testing.T) {
	sink := NewSink(nil)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
	})
	rec := connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-1"},
		CapturedBy: "connector:gmail",
		ThreadKey:  strings.Repeat("r", maxIndexedHeaderChars+1),
	}

	if _, err := sink.Upsert(ctx, rec); !errors.Is(err, connector.ErrSkip) {
		t.Errorf("Upsert = %v, want an error wrapping connector.ErrSkip", err)
	}
}

// The bound exists because Postgres refuses to index a value it cannot fit in a
// btree entry, and every value below is a header the sender wrote. A refusal
// arrives as a write error deep inside the page transaction, which the capture
// error vocabulary cannot classify — so the run ends and every message behind
// the offending one is lost. Skipping the one record is the whole remedy.
func TestARecordTooLargeToIndexIsSkippedRatherThanFailingItsPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  connector.NormalizedRecord
	}{
		{
			// The observed failure: a References header with no usable
			// separator yields the whole header as the thread's identity, and
			// thread_key carries three btree indexes.
			name: "a thread key past the bound",
			rec: connector.NormalizedRecord{
				NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-1"},
				ThreadKey:  strings.Repeat("r", maxIndexedHeaderChars+1),
			},
		},
		{
			name: "a source id past the bound",
			rec: connector.NormalizedRecord{
				NaturalKey: connector.NaturalKey{
					SourceSystem: connector.EmailSourceSystem,
					SourceID:     strings.Repeat("m", maxIndexedHeaderChars+1),
				},
			},
		},
		{
			name: "a counterparty address past the bound",
			rec: connector.NormalizedRecord{
				NaturalKey:   connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-2"},
				Counterparty: connector.Counterparty{Email: strings.Repeat("a", maxIndexedAddressChars+1) + "@acme.example"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := admitIndexableKeys(tc.rec)
			if !errors.Is(err, connector.ErrSkip) {
				t.Errorf("admitIndexableKeys = %v, want an error wrapping connector.ErrSkip — a hard error here ends the whole run", err)
			}
		})
	}
}

// The bound must not turn ordinary mail away. A value AT the limit is legal:
// off-by-one here would discard real correspondence, which is the failure this
// change exists to stop rather than to reproduce in the other direction.
func TestOrdinaryMailAndExactlyBoundedValuesAreAdmitted(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  connector.NormalizedRecord
	}{
		{
			name: "an ordinary message",
			rec: connector.NormalizedRecord{
				NaturalKey:   connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "CADq0J2y@mail.example"},
				ThreadKey:    "CADq0J2y@mail.example",
				Counterparty: connector.Counterparty{Email: "tuyen@acme.example"},
			},
		},
		{
			name: "values exactly at the bound",
			rec: connector.NormalizedRecord{
				NaturalKey: connector.NaturalKey{
					SourceSystem: connector.EmailSourceSystem,
					SourceID:     strings.Repeat("m", maxIndexedHeaderChars),
				},
				ThreadKey:    strings.Repeat("r", maxIndexedHeaderChars),
				Counterparty: connector.Counterparty{Email: strings.Repeat("a", maxIndexedAddressChars)},
			},
		},
		{
			// A record naming no counterparty and no thread is the common
			// shape for a calendar or channel record; absent is not oversized.
			name: "a record carrying neither a thread nor an address",
			rec: connector.NormalizedRecord{
				NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-3"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := admitIndexableKeys(tc.rec); err != nil {
				t.Errorf("admitIndexableKeys = %v, want nil — this record indexes fine", err)
			}
		})
	}
}

// Multi-byte headers are the case a byte bound and a rune bound disagree about.
// The index limit Postgres enforces is in BYTES, so the guard has to count them:
// a rune count would admit a value three times over the byte limit and hand the
// page the very write error the guard exists to prevent.
func TestTheBoundCountsBytesNotRunes(t *testing.T) {
	// Three bytes per rune in UTF-8, so this is under the bound by runes and
	// well over it by bytes.
	wide := strings.Repeat("あ", maxIndexedHeaderChars-1)

	rec := connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "m-4"},
		ThreadKey:  wide,
	}
	if err := admitIndexableKeys(rec); !errors.Is(err, connector.ErrSkip) {
		t.Errorf("admitIndexableKeys = %v, want ErrSkip: %d runes is %d bytes, past the %d-byte bound",
			err, len([]rune(wide)), len(wide), maxIndexedHeaderChars)
	}
}
