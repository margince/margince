// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// A seat refused an edit on a record must not be able to change how that
// record is filed either. ApplyTag and RemoveTag write to the
// RECORD — tags.go's own comment says so — so their row gate has to be the
// record's WRITE authority, not merely whether the caller may see it. A rep's
// read of a contact is workspace-wide (person is an identity table); their
// write is team-scoped. That gap between the two is exactly what a plain
// visibility probe cannot tell apart from write authority, and exactly what
// the unit lane (tagapplyauthz_test.go) cannot reach — it proves the OBJECT
// grant pair with a nil pool, never the ROW scope, which only a real
// Postgres row-scope predicate can arbitrate.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	collectionsmod "github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createTagViaStore is createTag's store-path counterpart: this package's
// createTag (tagvocab_integration_test.go) goes through the HTTP handler and
// answers a string id; the tests here exercise the store directly and need
// the typed ids.TagID its own methods take.
func createTagViaStore(t *testing.T, e *integration.Env, tags *collectionsmod.Store, name string) ids.TagID {
	t.Helper()
	tag, err := tags.NewTag(e.Admin(), name, "")
	if err != nil {
		t.Fatalf("seeding the tag %q: %v", name, err)
	}
	return ids.From[ids.TagKind](tag.TagID)
}

func TestATagWriteOnAForeignOwnedRecordIsRefused(t *testing.T) {
	e := integration.Setup(t)
	tags := collectionsmod.NewStore(e.DB())

	own := e.SeedPerson(t, "Own", &e.Rep1)
	foreign := e.SeedPerson(t, "Foreign", &e.Rep3)
	tagID := createTagViaStore(t, e, tags, "Key Account")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// The admit arm first: this seat CAN read the foreign contact (person is
	// read-all), which is the exact condition an object grant alone cannot
	// tell apart from write authority. Without this arm, the refusals below
	// would prove nothing — they would pass just as happily against a path
	// that refuses everything.
	if _, err := e.People.GetPerson(rep, integration.PersonIDOf(foreign), storekit.LiveOnly); err != nil {
		t.Fatalf("reading a foreign contact: %v, want success", err)
	}

	// The paired allow arm: the same seat, on a record it DOES own, may tag
	// and untag freely. Without this, a refusal that denied every tag write —
	// own record included — would pass this test just as happily as the fix.
	if _, err := tags.ApplyTag(rep, tagID, "person", own); err != nil {
		t.Errorf("applying a tag to this seat's own contact: %v, want success", err)
	}
	if err := tags.RemoveTag(rep, tagID, "person", own); err != nil {
		t.Errorf("removing a tag from this seat's own contact: %v, want success", err)
	}

	if _, err := tags.ApplyTag(rep, tagID, "person", foreign); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("applying a tag to a record this seat may only read → %v, want ErrPermissionDenied", err)
	}

	// Give removal something to refuse: an actor who does hold write
	// authority over the record applies the tag first.
	if _, err := tags.ApplyTag(e.Admin(), tagID, "person", foreign); err != nil {
		t.Fatalf("admin applying the tag: %v", err)
	}
	if err := tags.RemoveTag(rep, tagID, "person", foreign); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("removing a tag from a record this seat may only read → %v, want ErrPermissionDenied", err)
	}

	// The refusal must be loud, not a silent no-op: the tag is still there.
	rt, err := tags.RecordTagsFor(e.Admin(), "person", foreign)
	if err != nil {
		t.Fatalf("reading the record's tags: %v", err)
	}
	if len(rt.Data) != 1 {
		t.Errorf("record carries %d tags after the refused removal, want 1 — "+
			"the refusal must not have silently removed it anyway", len(rt.Data))
	}
}

// EnsureWritableLive is the LIVE half of the gate, not merely the write half —
// a mutation that swapped it for plain EnsureWritable (no archived_at filter)
// passed every other test in this file, because every foreign-owned fixture
// above is refused before liveness is ever asked. An archived record is nobody
// to tag, own or not: the record is frozen, and every taggable type reads
// workspace-wide (tableclass.go) so this is the only shape of not-found a tag
// write can still produce.
func TestATagWriteOnAnArchivedRecordIsRefused(t *testing.T) {
	e := integration.Setup(t)
	tags := collectionsmod.NewStore(e.DB())

	own := e.SeedPerson(t, "Own", &e.Rep1)
	tagID := createTagViaStore(t, e, tags, "Archived Target")

	if _, err := e.People.ArchivePerson(e.Admin(), integration.PersonIDOf(own), nil); err != nil {
		t.Fatalf("archiving the fixture: %v", err)
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	if _, err := tags.ApplyTag(rep, tagID, "person", own); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("applying a tag to an archived record this seat owns → %v, want ErrNotFound", err)
	}
	if err := tags.EnsureTaggable(rep, "person", own); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("EnsureTaggable on an archived record → %v, want ErrNotFound", err)
	}

	// Removal must refuse the same way, not treat "already gone" as the
	// idempotent no-tagging-here case: the record is frozen, and that is a
	// different fact than the tagging never having existed.
	if err := tags.RemoveTag(e.Admin(), tagID, "person", own); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("removing from an archived record → %v, want ErrNotFound", err)
	}
}

// An ownerless record (a connector-imported contact nobody has claimed) reads
// as workspace-shared, but ensureWriteAuthority's own doc says unowned is
// nobody's to write below RowScopeAll — the same rule PATCH already enforces
// on this row. This is the one real behavior change the fix carries: before
// it, EnsureLinkTarget's pure visibility probe let ANY bounded seat tag an
// ownerless row; now nobody below RowScopeAll can, matching every other
// mutation on the same table.
func TestATagWriteOnAnOwnerlessRecordIsRefusedBelowRowScopeAll(t *testing.T) {
	e := integration.Setup(t)
	tags := collectionsmod.NewStore(e.DB())

	unowned := e.SeedPerson(t, "Unclaimed", nil)
	tagID := createTagViaStore(t, e, tags, "Ownerless Target")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	if _, err := e.People.GetPerson(rep, integration.PersonIDOf(unowned), storekit.LiveOnly); err != nil {
		t.Fatalf("reading an unowned contact: %v, want success — read-all covers ownerless rows too", err)
	}

	if _, err := tags.ApplyTag(rep, tagID, "person", unowned); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a bounded seat applying a tag to an unowned record → %v, want ErrPermissionDenied", err)
	}
	if err := tags.EnsureTaggable(rep, "person", unowned); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("EnsureTaggable on an unowned record → %v, want ErrPermissionDenied", err)
	}

	// Give the bounded seat's RemoveTag refusal something to remove: an
	// unbounded actor applies the tag first.
	if _, err := tags.ApplyTag(e.Admin(), tagID, "person", unowned); err != nil {
		t.Fatalf("an unbounded actor applying a tag to an unowned record: %v, want success", err)
	}
	if err := tags.RemoveTag(rep, tagID, "person", unowned); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a bounded seat removing a tag from an unowned record → %v, want ErrPermissionDenied", err)
	}

	// The allow arm: an unbounded actor (RowScopeAll) still may, so this is a
	// row-scope narrowing and not the table having quietly become untaggable
	// for everyone.
	if err := tags.RemoveTag(e.Admin(), tagID, "person", unowned); err != nil {
		t.Errorf("an unbounded actor removing a tag from an unowned record: %v, want success", err)
	}
}

// EnsureTaggable is the same invariant, asked one step earlier — the
// apply-by-name path's own pre-flight before it resolves a tag name, so a
// failed apply leaves no live word behind. It must refuse exactly what the
// real write refuses, or the pre-flight admits a caller the write itself
// then turns away — which still ends in a refusal, but only after
// resolving the tag name the pre-flight exists to gate ahead of.
func TestEnsureTaggableRefusesTheSameForeignOwnedRecordApplyDoes(t *testing.T) {
	e := integration.Setup(t)
	tags := collectionsmod.NewStore(e.DB())

	own := e.SeedPerson(t, "Own", &e.Rep1)
	foreign := e.SeedPerson(t, "Foreign", &e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if err := tags.EnsureTaggable(rep, "person", own); err != nil {
		t.Errorf("EnsureTaggable on this seat's own contact: %v, want success", err)
	}
	if err := tags.EnsureTaggable(rep, "person", foreign); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("EnsureTaggable on a record this seat may only read → %v, want ErrPermissionDenied", err)
	}
}

// The importer's transactional door carries the same row gate. It is
// reached with no screen behind it, so a gap here is the more dangerous of
// the two: nothing on screen would show the tag landing on a record its
// approver could not otherwise touch.
func TestTheImportersTagWriteRefusesAForeignOwnedRecordToo(t *testing.T) {
	e := integration.Setup(t)
	tags := collectionsmod.NewStore(e.DB())

	own := e.SeedPerson(t, "Own", &e.Rep1)
	foreign := e.SeedPerson(t, "Foreign", &e.Rep3)
	tagID := createTagViaStore(t, e, tags, "Imported")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	tx, err := e.Pool.Begin(rep)
	if err != nil {
		t.Fatalf("opening a transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(rep); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the test transaction: %v", err)
		}
	}()

	// The paired allow arm: this same transactional door lets the seat tag a
	// record it owns. Without this, a regression that denied every
	// ApplyTagTx call — own record included — would pass just as happily.
	if _, err := tags.ApplyTagTx(rep, tx, tagID, "person", own); err != nil {
		t.Errorf("the importer's apply on this seat's own contact: %v, want success", err)
	}
	if _, err := tags.ApplyTagTx(rep, tx, tagID, "person", foreign); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the importer's apply on a record this seat may only read → %v, want ErrPermissionDenied", err)
	}
}
