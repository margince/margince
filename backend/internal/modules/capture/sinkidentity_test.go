// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which records carry a cross-door identity, and which deliberately do not.
//
// The sink files a record under a shared identity so the OTHER ingestion door
// resolves onto the same row. Getting this wrong is silent in both directions: a
// missing identity lands a duplicate nobody traces back here, and an identity
// claimed too eagerly binds two records that are not one thing.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// testKeyer stands in for activities.MeetingIdentityKey. The sink may not import
// that module, and what is under test here is WHICH records get a key rather
// than how the key is spelled — that agreement is proven in the owning module by
// TestOneOccurrenceIsOneIdentityHoweverItsStartIsSpelled.
func testKeyer(series, occurrence string) string { return series + "/" + occurrence }

func identitySink() *Sink {
	return (&Sink{}).WithMessageIdentity("mail", "meeting", testKeyer, nil, nil)
}

// A captured email is identified by its Message-ID, straight off the natural
// key: every transport agrees on that value, so no composition is needed.
func TestAMailRecordIsIdentifiedByItsMessageID(t *testing.T) {
	kind, key := identitySink().identityOfRecord(connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{
			SourceSystem: connector.EmailSourceSystem,
			SourceID:     "msg-1@counterparty.example",
		},
	})
	if kind != "mail" || key != "msg-1@counterparty.example" {
		t.Fatalf("a mail record identified as (%q, %q), want the mail kind and its Message-ID", kind, key)
	}
}

// A captured meeting is identified by its SERIES plus its OCCURRENCE, never by
// the provider's own event id — two calendars number one meeting differently,
// which is exactly the duplicate this exists to prevent.
func TestAMeetingRecordIsIdentifiedByItsSeriesAndOccurrence(t *testing.T) {
	start := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	kind, key := identitySink().identityOfRecord(connector.NormalizedRecord{
		NaturalKey:        connector.NaturalKey{SourceSystem: "gcal", SourceID: "goog-evt-1"},
		CrossDoorIdentity: connector.CrossDoorIdentity{Series: "series-42@google.com", Occurrence: start},
	})
	if kind != "meeting" {
		t.Fatalf("a meeting record identified as kind %q, want the meeting kind", kind)
	}
	if key != "series-42@google.com/2026-09-23T08:00:00Z" {
		t.Fatalf("a meeting keyed as %q, want the series and the occurrence", key)
	}
	// The provider's own event id is NOT the identity, and must not appear in it:
	// a colleague's calendar numbers the same meeting something else.
	if key == "goog-evt-1" {
		t.Fatal("the meeting was keyed on the provider's own event id")
	}
}

// A series with no occurrence identifies NOTHING.
//
// A recurring event shares one UID across every meeting in it, so a key on the
// series alone would resolve all fifty-two occurrences of a weekly call onto the
// first one — silently merging meetings that are not the same meeting.
func TestASeriesWithoutAnOccurrenceIdentifiesNothing(t *testing.T) {
	_, key := identitySink().identityOfRecord(connector.NormalizedRecord{
		NaturalKey:        connector.NaturalKey{SourceSystem: "gcal", SourceID: "goog-evt-1"},
		CrossDoorIdentity: connector.CrossDoorIdentity{Series: "series-42@google.com"},
	})
	if key != "" {
		t.Fatalf("a series with no occurrence keyed as %q, want no identity", key)
	}
}

// A record stating no identity carries none, and one is never invented. It
// captures under its natural key exactly as it always did.
func TestARecordStatingNoIdentityCarriesNone(t *testing.T) {
	_, key := identitySink().identityOfRecord(connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{SourceSystem: "gcal", SourceID: "goog-evt-1"},
	})
	if key != "" {
		t.Fatalf("a record stating no identity keyed as %q, want none", key)
	}
}

// A sink with no identity seams files nothing, which is the behaviour that
// predates cross-door identity: every record keys on its natural key alone.
func TestASinkWithoutTheSeamsFilesNoIdentity(t *testing.T) {
	start := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	for _, rec := range []connector.NormalizedRecord{
		{NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "msg-1@x.example"}},
		{CrossDoorIdentity: connector.CrossDoorIdentity{Series: "series-42@google.com", Occurrence: start}},
	} {
		if _, key := (&Sink{}).identityOfRecord(rec); key != "" {
			t.Fatalf("a sink with no seams keyed a record as %q, want no identity", key)
		}
	}
}
