// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// What each caller does when a declaration cannot be fingerprinted.
//
// The refusal itself is fingerprint_test.go's subject. These are the arms that
// carry it — a caller that swallowed the error would stamp or compare against a
// digest that names a projection this code cannot produce, and every row under
// it would read as stale forever.

import (
	"context"
	"strings"
	"testing"
)

// unencodableDeclaration is a mapping carrying a value JSON cannot encode. A
// channel is the cheapest to write down; the real one would be a func or a
// cyclic structure reaching a declaration through decoded JSON that was not
// decoded from JSON.
func unencodableDeclaration() ObjectMapping {
	return ObjectMapping{
		Source: "contacts", Target: "person", ExternalKey: "hs_object_id",
		Const: map[string]any{"unencodable": make(chan int)},
	}
}

// A child attribute JSON cannot encode is refused, and names the field.
//
// Const has its own case one file over; this is the second site, and it is a
// separate arm rather than the same one — a mapping whose Const encodes and
// whose Child.Attrs does not would pass the first check and reach the second.
func TestAnUnencodableChildAttributeIsRefused(t *testing.T) {
	m := ObjectMapping{
		Source: "contacts", Target: "person", ExternalKey: "hs_object_id",
		Fields: []FieldMapping{{
			From: []string{"email"}, To: "person_email.email", Kind: TargetChild,
			Child: &ChildRow{Attrs: map[string]any{"unencodable": make(chan int)}},
		}},
	}

	digest, err := Fingerprint(m)

	if err == nil {
		t.Fatalf("a child attribute JSON cannot encode answered the digest %q", digest)
	}
	// The field INDEX, because a declaration has many and the reader is
	// whoever just edited one.
	if !strings.Contains(err.Error(), "field 0") {
		t.Errorf("the refusal reads %q and does not say which field carries the value", err)
	}
}

// StaleProjections answers the refusal rather than comparing against a digest
// it does not have.
//
// The store is nil on purpose: the refusal has to come BEFORE any database
// work, so a test that reached one would be proving something else.
//
// The error's IDENTITY is asserted, not merely its presence, and that is what
// makes this evidence. A nil store fails on its own the moment anything touches
// it, so "an error came back" is true whether or not the fingerprint was
// checked — swallowing the refusal still answers an error, from one line
// further down. Naming the key that could not be encoded is what separates the
// two.
func TestStaleProjectionsRefusesADeclarationItCannotFingerprint(t *testing.T) {
	store := NewMirrorStore(nil, noOwnerEmailsForUnitTest{})

	ids, err := store.StaleProjections(context.Background(), unencodableDeclaration(), 10)

	if err == nil {
		t.Fatal("StaleProjections answered a stale set for a declaration it cannot fingerprint — the " +
			"comparison it makes is against that digest, so the answer describes nothing")
	}
	if !strings.Contains(err.Error(), "unencodable") {
		t.Errorf("it refused with %q, which is not the fingerprint refusal — the declaration was never "+
			"checked and this failed somewhere further down, so the check above proves nothing", err)
	}
	if len(ids) != 0 {
		t.Errorf("it answered %d id(s) alongside the refusal; a caller taking the value would re-fetch them", len(ids))
	}
}
