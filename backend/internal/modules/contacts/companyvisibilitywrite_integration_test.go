// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Company visibility as an ORDINARY field, over a real Postgres.
//
// The company column had no door at all. Capture mints a company owner-scoped
// from a message nothing has judged yet, and the only way out was a sender
// verdict — so a company the classifier never asked about, or judged wrong,
// stayed private to its mailbox owner with nothing a human could press. These
// tests hold the new rule to the database rather than to a rendered predicate:
// anybody the write gate admits moves the field, in either direction.
//
// Deliberately the same cases as visibilitywrite_integration_test.go beside
// them, because the two columns are meant to behave the same way. Where an
// answer differs, that difference is the thing to look at.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureCompany writes one company exactly as connector auto-create does:
// owned by e.owner, at the visibility the caller names, unpromoted.
func (e *privacyEnv) captureCompany(t *testing.T, visibility string) ids.CompanyID {
	t.Helper()
	id := ids.New[ids.CompanyKind]()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company (id, display_name, owner_id, source, captured_by, visibility)
			VALUES ($1, 'Captured GmbH', $2, 'gmail:seed', 'connector:gmail', $3)`,
			id, e.owner, visibility)
		return err
	}); err != nil {
		t.Fatalf("seeding a %s company: %v", visibility, err)
	}
	return id
}

// companyVisibilityOf reads the column straight, because what is under test is
// what the write left in the row.
func (e *privacyEnv) companyVisibilityOf(t *testing.T, id ids.CompanyID) string {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	var got string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT visibility FROM company WHERE id = $1`, id).Scan(&got)
	}); err != nil {
		t.Fatalf("reading visibility of company %s: %v", id, err)
	}
	return got
}

func (e *privacyEnv) companyOwnerOf(t *testing.T, id ids.CompanyID) *ids.UUID {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	var got *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT owner_id FROM company WHERE id = $1`, id).Scan(&got)
	}); err != nil {
		t.Fatalf("reading owner of company %s: %v", id, err)
	}
	return got
}

// canReadCompany answers whether one caller's GetCompany returns the row,
// failing the test on any error that is not the existence-hiding 404.
func (e *privacyEnv) canReadCompany(t *testing.T, id ids.CompanyID, user ids.UUID, scope principal.RowScope) bool {
	t.Helper()
	_, err := e.store.GetCompany(e.as(user, scope), id, storekit.LiveOnly)
	switch {
	case err == nil:
		return true
	case errors.Is(err, apperrors.ErrNotFound):
		return false
	default:
		t.Fatalf("reading company %s: %v", id, err)
		return false
	}
}

// TestAWorkspaceCompanyCanBeMadePrivate is the direction that did not exist
// for a company at all.
func TestAWorkspaceCompanyCanBeMadePrivate(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.captureCompany(t, "workspace")
	if !e.canReadCompany(t, published, e.teammate, principal.RowScopeTeam) {
		t.Fatal("a workspace company is not readable by a teammate — the fixture is wrong")
	}

	if _, err := e.store.UpdateCompany(e.as(e.owner, principal.RowScopeOwn), published,
		UpdateCompanyInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("making a published company private: %v", err)
	}

	if got := e.companyVisibilityOf(t, published); got != "owner" {
		t.Errorf("visibility = %q, want owner", got)
	}
	if e.canReadCompany(t, published, e.teammate, principal.RowScopeTeam) {
		t.Error("a teammate still reads a company its owner has just made private")
	}
}

// TestACapturedCompanyCanBePublished is the door the product was missing: the
// owner of a capture-private company publishing it themselves.
func TestACapturedCompanyCanBePublished(t *testing.T) {
	e := setupCapturePrivacy(t)
	captured := e.captureCompany(t, "owner")
	if e.canReadCompany(t, captured, e.teammate, principal.RowScopeTeam) {
		t.Fatal("a captured company is readable by a teammate — the fixture is wrong")
	}

	if _, err := e.store.UpdateCompany(e.as(e.owner, principal.RowScopeOwn), captured,
		UpdateCompanyInput{Visibility: visibility("workspace")}); err != nil {
		t.Fatalf("publishing a captured company: %v", err)
	}

	if got := e.companyVisibilityOf(t, captured); got != "workspace" {
		t.Errorf("visibility = %q, want workspace", got)
	}
	if !e.canReadCompany(t, captured, e.teammate, principal.RowScopeTeam) {
		t.Error("a teammate cannot read a company that has just been published")
	}
}

// TestNarrowingACompanyKeepsItsOwnOwner is the trap the field would otherwise
// set. company.owner_id is nullable while visibility independently says
// 'owner', and the row-scope arm reads the pair as
// `visibility <> 'owner' OR owner_id = me` — so an owner-scoped row naming
// nobody is readable by NOBODY, the caller included.
//
// A colleague narrowing somebody else's company must not be handed the row:
// whose company it becomes is a decision, not a side effect of pressing a
// button.
func TestNarrowingACompanyKeepsItsOwnOwner(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.captureCompany(t, "workspace")

	if _, err := e.store.UpdateCompany(e.as(e.teammate, principal.RowScopeTeam), published,
		UpdateCompanyInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("narrowing a colleague's company: %v", err)
	}

	owner := e.companyOwnerOf(t, published)
	if owner == nil {
		t.Fatal("a company made private carries no owner — nobody can read it")
	}
	if *owner != e.owner {
		t.Errorf("owner = %s, want the row's existing owner %s — narrowing must not reassign it",
			*owner, e.owner)
	}
	if !e.canReadCompany(t, published, e.owner, principal.RowScopeOwn) {
		t.Error("the owner cannot read their own company after it was made private")
	}
}

// TestNarrowingAnUnownedCompanyIsRefused holds the rule that keeps a record
// from being destroyed rather than made private.
//
// An unowned company narrowed to 'owner' satisfies neither arm of the
// row-scope predicate and becomes invisible to every seat, including the one
// that would repair it. Refused by field, not by constraint name: the caller
// is told which field to supply.
func TestNarrowingAnUnownedCompanyIsRefused(t *testing.T) {
	e := setupCapturePrivacy(t)
	unowned := e.seedCompany(t)
	id := ids.ID[ids.CompanyKind]{UUID: unowned}

	// An unbounded seat, because the ordinary write gate treats an unowned row
	// as nobody's to change — this is the caller that can actually reach it.
	_, err := e.store.UpdateCompany(e.as(e.admin, principal.RowScopeAll), id,
		UpdateCompanyInput{Visibility: visibility("owner")})
	var required *RequiredFieldError
	if !errors.As(err, &required) {
		t.Fatalf("narrowing an unowned company: err = %v, want a required-field refusal naming owner_id", err)
	}
	if required.Field != "owner_id" {
		t.Errorf("refused field = %q, want owner_id", required.Field)
	}
	if got := e.companyVisibilityOf(t, id); got != "workspace" {
		t.Errorf("visibility = %q after a refused narrowing, want workspace — the refusal must not have written", got)
	}
}

// TestAStaleCompanyVisibilityWriteIsRefused is the race the column cannot
// tolerate.
//
// updateCompanyInTx reads the row and decides admission BEFORE it locks it: an
// unconditional patch takes its FOR UPDATE inside ApplyGuarded. So a write
// admitted while the company was public can land after its owner has just made
// it private, re-publishing a record somebody deliberately closed and writing
// `workspace -> workspace` into the audit, because the before-image came from
// the stale read.
//
// This drives the guard directly rather than trying to interleave two
// transactions: refuseStaleCompanyVisibility is handed the before-image a
// racing caller would have carried — the row as it looked while still public —
// and must refuse it now that the stored row says otherwise.
func TestAStaleCompanyVisibilityWriteIsRefused(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.captureCompany(t, "workspace")
	ctx := e.as(e.owner, principal.RowScopeOwn)

	// The row as the racing caller read it: still public.
	stale, err := e.store.GetCompany(ctx, published, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the company: %v", err)
	}

	// Its owner makes it private in between.
	if _, err := e.store.UpdateCompany(ctx, published,
		UpdateCompanyInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("making it private: %v", err)
	}

	// The racing write reaches the guard carrying the stale before-image.
	err = e.store.tx(ctx, func(tx pgx.Tx) error {
		return refuseStaleCompanyVisibility(ctx, tx, published, stale)
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a write built on a stale read: err = %v, want conflict — it would have "+
			"re-published a company its owner had just made private", err)
	}

	// And the guard admits a write whose before-image is current.
	fresh, err := e.store.GetCompany(ctx, published, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the company: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return refuseStaleCompanyVisibility(ctx, tx, published, fresh)
	}); err != nil {
		t.Errorf("a write built on a current read was refused: %v", err)
	}
}

// TestCapturePrivacyStillHidesACompanyFromTheNewDoor is the security question
// the change had to answer.
//
// The two gates compose: a capture-private row is invisible to every other
// seat under the row-scope arm, so EnsureWritable cannot find it and the patch
// answers NOT FOUND rather than a permission error — existence stays hidden.
// Neither a teammate nor an admin can publish somebody else's private company.
func TestCapturePrivacyStillHidesACompanyFromTheNewDoor(t *testing.T) {
	e := setupCapturePrivacy(t)
	captured := e.captureCompany(t, "owner")

	for _, reader := range []struct {
		name  string
		user  ids.UUID
		scope principal.RowScope
	}{
		{"a teammate", e.teammate, principal.RowScopeTeam},
		{"an admin", e.admin, principal.RowScopeAll},
	} {
		_, err := e.store.UpdateCompany(e.as(reader.user, reader.scope), captured,
			UpdateCompanyInput{Visibility: visibility("workspace")})
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s publishing somebody else's capture-private company: err = %v, want not found",
				reader.name, err)
		}
	}
	if got := e.companyVisibilityOf(t, captured); got != "owner" {
		t.Errorf("visibility = %q, want owner — no refused write may have landed", got)
	}
}

// TestAuthorizationIsRecheckedUnderTheRowLock is the defect a review found in
// the first cut of this feature, kept as a test because the guard looked
// complete without it.
//
// updateCompanyInTx asks auth.EnsureWritable at the top, before the name lock
// and before the row is read. Under READ COMMITTED a privatization committing
// in that window is invisible to a guard that only compares the column to what
// the caller was shown: the caller's own before-image is read after the commit
// too, so both sides say 'owner', they match, and a colleague who may no
// longer see the record at all is admitted to re-publish it.
//
// Driven here as the real interleaving rather than argued: authorize while the
// company is public, let the owner privatize and COMMIT, then carry on exactly
// as the update path does. The refusal must be the existence-hiding one — a
// conflict would tell a caller who can no longer read the row that it is still
// there.
func TestAuthorizationIsRecheckedUnderTheRowLock(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.captureCompany(t, "workspace")
	colleague := e.as(e.teammate, principal.RowScopeTeam)

	active, err := e.store.activeColumns(colleague, "company")
	if err != nil {
		t.Fatalf("reading the active columns: %v", err)
	}

	err = e.store.tx(colleague, func(tx pgx.Tx) error {
		// Admitted while the company is still everybody's.
		if err := auth.EnsureWritable(colleague, tx, "company", published.UUID); err != nil {
			return err
		}

		// The owner closes it, in their own transaction, and commits.
		if _, err := e.store.UpdateCompany(e.as(e.owner, principal.RowScopeOwn), published,
			UpdateCompanyInput{Visibility: visibility("owner")}); err != nil {
			t.Fatalf("the owner privatizing mid-race: %v", err)
		}

		// From here the colleague proceeds as updateCompanyInTx does.
		current, err := readCompany(colleague, tx, published, storekit.LiveOnly, active)
		if err != nil {
			return err
		}
		return refuseStaleCompanyVisibility(colleague, tx, published, current)
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("republishing a company privatized after admission: err = %v, want not found", err)
	}

	if got := e.companyVisibilityOf(t, published); got != "owner" {
		t.Errorf("visibility = %q, want owner — the owner's decision must stand", got)
	}
}
