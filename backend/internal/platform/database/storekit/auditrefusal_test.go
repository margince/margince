// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The chokepoint: an update that cannot say what it changed from does not reach
// the table.
//
// auditbeforeimage_test.go is the census beside this, and it is the weaker half
// by construction. It walks the AST, so it cannot resolve a call made through an
// interface, a stored field or a closure, nor a verb held in a parameter or read
// from a row at run time — and a site it cannot judge is a site it must ratify.
// This is what actually holds those.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUpdateWithNoBeforeImageIsRefusedBeforeItReachesTheDatabase(t *testing.T) {
	// The transaction is checked as well as the error: a refusal that arrived
	// AFTER the insert would still return an error, and would still have written
	// the row this rule exists to keep out of the table.
	tx := &fakeTx{}
	_, err := Audit(auditingContext(), tx, "update", "person", ids.NewV7(), nil,
		map[string]any{"full_name": "Greta Machine"})
	if err == nil {
		t.Fatal("an update with no before-image was accepted")
	}
	if tx.execSQL != "" {
		t.Errorf("the refusal came too late; the row was already sent: %s", tx.execSQL)
	}
	if !strings.Contains(err.Error(), "before") {
		t.Errorf("the refusal does not name what is missing: %v", err)
	}
}

// The typed-nil case, which is the one a `before == nil` test would miss. A
// store that builds its image in a map[string]any and leaves it nil hands the
// seam an interface carrying a typed nil: it reaches the column as SQL NULL
// exactly as an untyped nil does, while `== nil` reads it as present.
func TestAnUpdateWithATypedNilBeforeImageIsRefusedToo(t *testing.T) {
	var image map[string]any
	tx := &fakeTx{}
	_, err := Audit(auditingContext(), tx, "update", "person", ids.NewV7(), image,
		map[string]any{"full_name": "Greta Machine"})
	if err == nil {
		t.Fatal("a typed nil before-image was accepted; the column would have stored SQL NULL")
	}
	if tx.execSQL != "" {
		t.Errorf("the refusal came too late; the row was already sent: %s", tx.execSQL)
	}
}

// An EMPTY image is not an absent one. A writer saying "these fields held
// nothing" has answered the question; refusing it would make the honest answer
// unsayable.
func TestAnUpdateWithAnEmptyBeforeImageIsAccepted(t *testing.T) {
	tx := &fakeTx{}
	if _, err := Audit(auditingContext(), tx, "update", "person", ids.NewV7(),
		map[string]any{}, map[string]any{"full_name": "Greta"}); err != nil {
		t.Fatalf("an empty before-image was refused: %v", err)
	}
}

// The declared door for a write with no prior state stays open, or the refusal
// would make recording an occurrence impossible.
func TestAnUpdateThroughAuditEventIsAccepted(t *testing.T) {
	tx := &fakeTx{}
	if _, err := AuditEvent(auditingContext(), tx, "update", "webhook_subscription", ids.NewV7(),
		map[string]any{"signing_secret_rotated": true}); err != nil {
		t.Fatalf("an occurrence-shaped update was refused: %v", err)
	}
}

// Only `update` is judged. An archive records no before-image and is left alone:
// un-archiving is not a replay of one but a per-type decision about what the
// archive took down with it.
func TestAVerbOtherThanUpdateMayCarryNoBeforeImage(t *testing.T) {
	for _, action := range []string{"create", "archive", "erase", "restrict", "promote"} {
		t.Run(action, func(t *testing.T) {
			tx := &fakeTx{}
			if _, err := Audit(auditingContext(), tx, action, "person", ids.NewV7(), nil,
				map[string]any{"full_name": "Greta"}); err != nil {
				t.Errorf("%s was refused for carrying no before-image: %v", action, err)
			}
		})
	}
}

// The evidence door refuses on the same terms. Two ways in would mean a writer
// could get past the rule by having something to say about the operation.
func TestTheEvidenceDoorRefusesOnTheSameTerms(t *testing.T) {
	tx := &fakeTx{}
	_, err := AuditWithEvidence(auditingContext(), tx, "update", "person", ids.NewV7(),
		nil, map[string]any{"full_name": "Greta"}, map[string]any{"source": "site_read"})
	if err == nil {
		t.Fatal("an update with evidence and no before-image was accepted")
	}
	if tx.execSQL != "" {
		t.Errorf("the refusal came too late; the row was already sent: %s", tx.execSQL)
	}
}

// A restore is update-shaped: it replaces field values a record already held.
// Binding the rule to the literal "update" alone would make the reversal verb
// the one way to write a field change that cannot say what it changed from.
func TestARestoreWithNoBeforeImageIsRefusedToo(t *testing.T) {
	tx := &fakeTx{}
	_, err := Audit(auditingContext(), tx, string(VerbRestore), "person", ids.NewV7(), nil,
		map[string]any{"full_name": "Greta Machine"})
	if err == nil {
		t.Fatal("a restore with no before-image was accepted")
	}
	if tx.execSQL != "" {
		t.Errorf("the refusal came too late; the row was already sent: %s", tx.execSQL)
	}
	if !strings.Contains(err.Error(), "before") {
		t.Errorf("the refusal does not name what is missing: %v", err)
	}
}
