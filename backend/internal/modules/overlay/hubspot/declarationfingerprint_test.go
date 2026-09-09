// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package hubspot

// The declarations this package ships can all be fingerprinted, and a record
// projected through one that cannot is refused rather than stamped.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/overlay"
)

// Every declaration this package ships fingerprints cleanly.
//
// This is what makes ProjectionFingerprints' panic a composition-time assertion
// rather than a live failure mode: the mappings are constants of this package,
// so if they all encode here they encode at boot too. It is also the reason the
// error arms in the callers are defensive rather than reachable — a declaration
// that could trip them would have to be written first, and this fails the
// moment one is.
//
// Derived from objectMappings, so a declaration added tomorrow is checked by
// being added.
func TestEveryShippedDeclarationCanBeFingerprinted(t *testing.T) {
	if len(objectMappings) == 0 {
		t.Fatal("this package declares no object mapping — the assertion below asks nothing, and " +
			"ProjectionFingerprints would answer an empty map every staleness verdict reads from")
	}
	seen := make(map[string]string, len(objectMappings))
	for _, m := range objectMappings {
		digest, err := overlay.Fingerprint(m)
		if err != nil {
			t.Errorf("the %s declaration cannot be fingerprinted (%v) — ProjectionFingerprints panics on "+
				"this at boot, which is where a declaration carrying an unencodable value should be found",
				m.Source, err)
			continue
		}
		if digest == "" {
			t.Errorf("the %s declaration fingerprints to the empty string, which every stamped row would "+
				"then carry and every comparison would match", m.Source)
		}
		// Two declarations sharing a digest would make one indistinguishable
		// from the other on a mirror row, so a re-projection could not tell
		// which produced it.
		if other, clash := seen[digest]; clash {
			t.Errorf("%s and %s fingerprint identically, so a row stamped with it names neither", m.Source, other)
		}
		seen[digest] = m.Source
	}
}

// A record projected through a declaration that cannot be fingerprinted is
// refused, never stamped.
//
// This is the arm that matters most of the four the refusal reaches: it is the
// one that would WRITE a bad fingerprint onto a mirror row, where every later
// staleness verdict reads it. A row stamped with a digest naming no projection
// reads as stale forever and re-fetches on every pass.
func TestARecordIsNotStampedWithAFingerprintThatCannotBeMade(t *testing.T) {
	// Baseline and its property are supplied so the projection gets PAST the
	// watermark check: without them the mapper refuses earlier, for a reason
	// that has nothing to do with fingerprinting, and this test would pass
	// while never reaching the arm it names.
	m := overlay.ObjectMapping{
		Source: objectClassContacts, Target: "person", ExternalKey: propHSObjectID,
		Baseline: baselineHSLastModifiedDate,
		Const:    map[string]any{"unencodable": make(chan int)},
	}
	raw := ObjectRecord{ID: "1", Properties: Property{baselineHSLastModifiedDate: "2026-01-02T03:04:05.000Z"}}

	rec, err := mapRecord(m, objectClassContacts, raw)

	if err == nil {
		t.Fatalf("a record projected through an unfingerprintable declaration came back stamped %q — "+
			"the mirror would hold a row naming a projection this code cannot produce", rec.ProjectionFingerprint)
	}
	if rec.ProjectionFingerprint != "" {
		t.Errorf("the refusal still carried the fingerprint %q; a caller ignoring the error would store it",
			rec.ProjectionFingerprint)
	}
	if !strings.Contains(err.Error(), "unencodable") {
		t.Errorf("the refusal reads %q and does not name the key that could not be encoded", err)
	}
}
