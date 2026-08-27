// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Two people reversing ONE audited link change, with the race FORCED rather than
// hoped for.
//
// The window is between the binding edge decision and the edge write: the
// decision reads the edge's version and the write pins it, and the two are
// separate transactions because the write is the people store's own. A test that
// starts two requests and lets the scheduler decide can pass as a sequential
// "the second one loses", which says nothing about a reverse overtaken INSIDE
// that window — the only place the defect lives. So the first reverser is held
// there on a channel while the second completes.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// restoreSeamFor is the executor as the server assembles it, over the test's own
// pool.
func restoreSeamFor(e *integration.Env) RestoreSeam {
	return NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool))
}

// reversalsOf counts the audit rows that name this entry as the one they put
// back. One entry reversed twice is two of them, which is the shape a lost race
// leaves behind.
func reversalsOf(t *testing.T, e *integration.Env, auditID ids.UUID) int {
	t.Helper()
	admin := e.Admin()
	var count int
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `
			SELECT count(*) FROM audit_log
			WHERE evidence ->> $1 = $2::text`, privacy.UndidAuditLogID, auditID).Scan(&count)
	}); err != nil {
		t.Fatalf("count what reverses %s: %v", auditID, err)
	}
	return count
}

// The overtaken reverse refuses, and the change it was putting back was put back
// exactly once.
//
// A CHANGE to the link rather than its creation, deliberately: the inverse of a
// create is an archive, and a second archive of an already-archived link would be
// refused as an absent row whatever guard was in place — so that case cannot tell
// a serialised path from an unserialised one. Replaying a field image twice
// succeeds twice unless something pins the version it was decided on.
func TestAnEdgeReverseOvertakenInsideItsDecisionWindowRefuses(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	personID := e.SeedPerson(t, "Ada Overtaken", nil)
	orgID := e.SeedOrg(t, "Overtaken GmbH", nil)
	person := ids.From[ids.PersonKind](personID)
	org := ids.From[ids.OrganizationKind](orgID)
	held, changed := "cto", "coo"
	edge, err := e.People.CreateRelationship(ctx, people.CreateRelationshipInput{
		Kind: "employment", PersonID: &person, OrganizationID: &org,
		Role: &held, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the link through the real writer: %v", err)
	}
	if _, err := e.People.UpdateRelationship(ctx, edge.ID,
		people.UpdateRelationshipInput{Role: &changed}); err != nil {
		t.Fatalf("change the link through the real writer: %v", err)
	}
	auditID := latestAuditRowID(t, e, edgeEntityType, edge.ID, "update")
	version := currentVersion(t, e, "person", personID)

	decided, release := make(chan struct{}), make(chan struct{})
	overtaken := restoreSeamFor(e)
	overtaken.afterEdgeDecision = func() {
		close(decided)
		<-release
	}
	refusal := make(chan error, 1)
	go func() {
		_, err := overtaken.Restore(ctx, "person", personID, auditID, version)
		refusal <- err
	}()

	<-decided
	// The whole second reverse — decision and write — inside the first's window.
	if _, err := restoreSeamFor(e).Restore(ctx, "person", personID, auditID, version); err != nil {
		t.Fatalf("the overtaking reverse: %v", err)
	}
	close(release)

	err = <-refusal
	if err == nil {
		t.Fatal("both reverses of one link change committed; the second decided and " +
			"wrote entirely inside the first's decision window, so the first wrote " +
			"onto a link nobody had judged in that state")
	}
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("the overtaken reverse answered %v, want a skew on the EDGE's own "+
			"version — neither record's version moves on an edge write, so nothing "+
			"else can serialise the two", err)
	}
	if count := reversalsOf(t, e, auditID); count != 1 {
		t.Errorf("%d rows say they put entry %s back, want 1", count, auditID)
	}
}
