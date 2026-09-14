// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a proof row may claim about what the subject was shown, refused before
// a connection is ever taken.
//
// These sit in the unit lane deliberately. The rule is enforced in admitRecord,
// which runs before s.db.Tx opens, and a nil pool is what PROVES that: were the
// refusal to move inside the transaction, or come to rest on a database
// constraint instead, these tests would panic rather than pass. Against a live
// pool they would go on passing and prove only that something, somewhere,
// said no.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// wordingCtx binds the actor the writer requires, and nothing else. The RBAC
// check runs before the wording rule, so a bare context would fail these tests
// on the wrong obligation and prove nothing about wording.
func wordingCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"contact": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestAGrantWithoutWordingIsRefused holds Art. 7(1) at the writer.
//
// It used to be recorded, with the literal 'recorded via API' standing in for
// the sentence nobody captured — a row that reads like evidence in a subject
// access export and demonstrates nothing.
func TestAGrantWithoutWordingIsRefused(t *testing.T) {
	store := NewStore(nil)

	_, err := store.Record(wordingCtx(), RecordInput{
		ContactID: ids.New[ids.ContactKind](),
		PurposeID: ids.New[ids.PurposeKind](),
		NewState:  "granted",
	})

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("recording a wordless grant returned %v, want a validation error", err)
	}
	if invalid.Field != "wording" {
		t.Errorf("refusal named field %q, want %q", invalid.Field, "wording")
	}
}

// TestWordingIsBoundedAtTheWriter keeps a proof row from becoming a payload.
func TestWordingIsBoundedAtTheWriter(t *testing.T) {
	store := NewStore(nil)
	huge := strings.Repeat("x", maxWordingRunes+1)

	_, err := store.Record(wordingCtx(), RecordInput{
		ContactID:  ids.New[ids.ContactKind](),
		PurposeID:  ids.New[ids.PurposeKind](),
		NewState:   "granted",
		PolicyText: &huge,
	})

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("an oversized wording returned %v, want a validation error — unchecked, one "+
			"caller stores a megabyte every later reader of this history is served in full", err)
	}
	if invalid.Field != "wording" {
		t.Errorf("refusal named field %q, want %q", invalid.Field, "wording")
	}
}

// TestTheWordingVersionIsBoundedToo covers the other half of the pair.
//
// consent_event_wording_pairs stores the two together, so a bound on the
// sentence alone leaves the same hole open one column across: the anonymous
// booking door sets the version, it is written verbatim, and the subject
// access export reads it back in full.
func TestTheWordingVersionIsBoundedToo(t *testing.T) {
	store := NewStore(nil)
	huge := strings.Repeat("v", maxVersionRunes+1)

	_, err := store.Record(wordingCtx(), RecordInput{
		ContactID:     ids.New[ids.ContactKind](),
		PurposeID:     ids.New[ids.PurposeKind](),
		NewState:      "granted",
		PolicyText:    &grantWording,
		PolicyVersion: &huge,
	})

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("an oversized version returned %v, want a validation error", err)
	}
	if invalid.Field != "policy_version" {
		t.Errorf("refusal named field %q, want %q", invalid.Field, "policy_version")
	}
}

// TestAWithdrawalStoresNoWordingEvenWhenGivenSome holds the rule at the WRITER
// rather than at the door.
//
// Fixtures pass one input for both states to keep a helper single-shaped, and
// admitRecord only refuses a grant that has none — it does not strip what a
// withdrawal supplies. Without wordingFor the row would record the sentence
// that accompanied a GRANT as though the contact had read it while opting out.
func TestAWithdrawalStoresNoWordingEvenWhenGivenSome(t *testing.T) {
	version := "v1"
	text, gotVersion := wordingFor(StateWithdrawn, &grantWording, &version)
	if text != nil || gotVersion != nil {
		t.Errorf("a withdrawal stored wording %v / version %v, want both nil: nothing is "+
			"demonstrated when somebody takes consent back", text, gotVersion)
	}
}

// TestAVersionWithoutTextIsNeverStored keeps the pair CHECK from being reached
// as a raw database error.
//
// consent_event_wording_pairs refuses one column without the other. A caller
// passing a version and no text used to satisfy validation and fail at the
// constraint, which surfaces as an opaque 500 rather than a refusal naming the
// field.
func TestAVersionWithoutTextIsNeverStored(t *testing.T) {
	version := "v1"
	text, gotVersion := wordingFor(StateGranted, nil, &version)
	if text != nil || gotVersion != nil {
		t.Errorf("stored version %v with text %v, want both nil so the pair CHECK is never "+
			"reached with half a row", gotVersion, text)
	}
}

// TestABlankVersionIsRefused — absent means "the writer picks the default";
// a caller-supplied empty string would store wording naming no version at all.
func TestABlankVersionIsRefused(t *testing.T) {
	blank := "   "
	err := requireBoundedVersion(&blank)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("a blank version returned %v, want a validation error", err)
	}
	if invalid.Field != "policy_version" {
		t.Errorf("refusal named field %q, want policy_version", invalid.Field)
	}
}
