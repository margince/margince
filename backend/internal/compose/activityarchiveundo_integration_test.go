// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func archivedSourceNote(t *testing.T, e *integration.Env) (ids.UUID, ids.UUID, ids.UUID) {
	t.Helper()
	contact := e.SeedContact(t, "Source contact", &e.AdminUser)
	row, _, err := e.Activities.LogActivity(machineCtx(e), activities.LogActivityInput{Kind: "note", Source: "import", SourceSystem: new("external-notes"), SourceID: new("note-1"), Subject: new("Source meeting notes"), Body: new("Original recorded content"), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(row.Id)
	if _, err := e.Activities.ArchiveActivity(machineCtx(e), ids.From[ids.ActivityKind](id), nil); err != nil {
		t.Fatal(err)
	}
	return id, latestAuditRowID(t, e, entityTypeActivity, id, actionArchive), contact
}

func TestActivityArchiveUndoPreservesSourceAndCanBeRedone(t *testing.T) {
	e := integration.Setup(t)
	id, archive, _ := archivedSourceNote(t, e)
	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeActivity, id, archive); !answer.Undoable {
		t.Fatalf("archive not reversible: %+v", answer)
	}
	if err := undoEntry(t, e, entityTypeActivity, id, archive); err != nil {
		t.Fatal(err)
	}
	row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.SourceSystem == nil || *row.SourceSystem != "external-notes" || row.SourceId == nil || *row.SourceId != "note-1" || row.Body == nil || *row.Body != "Original recorded content" {
		t.Fatal("undo changed capture identity or content")
	}
	if reversalsOf(t, e, archive) != 1 {
		t.Fatal("archive lacks one atomic reversal")
	}
	restored := latestAuditRowID(t, e, entityTypeActivity, id, actionRestore)
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=$1`, restored.String()); n != 1 {
		t.Fatalf("restore outbox count=%d", n)
	}
	if err := undoEntry(t, e, entityTypeActivity, id, restored); err != nil {
		t.Fatal(err)
	}
	if !isArchived(t, e, entityTypeActivity, id) {
		t.Fatal("redo did not archive activity")
	}
}

func TestActivityArchiveUndoRefusesStaleVersionAndMissingAuthority(t *testing.T) {
	e := integration.Setup(t)
	id, archive, _ := archivedSourceNote(t, e)
	version := currentVersion(t, e, entityTypeActivity, id)
	if _, err := restoreSeamFor(e).Restore(e.Admin(), entityTypeActivity, id, archive, version-1); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale undo: %v", err)
	}
	reader := e.As(e.AdminUser, nil, principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}, "contact": {Read: true}}})
	if answer := advisoryAnswer(reader, t, e, entityTypeActivity, id, archive); answer.Undoable || answer.Reason != string(ReasonNotWritableByCaller) {
		t.Fatalf("delete grant not required: %+v", answer)
	}
	err := e.DB().Tx(reader, func(tx pgx.Tx) error {
		_, err := e.Activities.RestoreActivityTx(reader, tx, ids.From[ids.ActivityKind](id), &version, unarchiveWith(nil))
		return err
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("native grant refusal: %v", err)
	}
	if !isArchived(t, e, entityTypeActivity, id) {
		t.Fatal("refused undo changed row")
	}
}

func TestActivityArchiveUndoCannotCrossErasure(t *testing.T) {
	e := integration.Setup(t)
	id, archive, contact := archivedSourceNote(t, e)
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contact, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if err := undoEntry(t, e, entityTypeActivity, id, archive); err == nil {
		t.Fatal("erased activity was restored")
	}
	if !isArchived(t, e, entityTypeActivity, id) {
		t.Fatal("erasure boundary changed")
	}
}

func TestActivityArchiveRestoreKeepsSelectedAudience(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Private notes", &e.AdminUser)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Private source"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.SetAudience(e.Admin(), id, activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.AdminUser}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), id, nil); err != nil {
		t.Fatal(err)
	}
	other := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	err = e.DB().Tx(other, func(tx pgx.Tx) error {
		_, err := e.Activities.RestoreActivityTx(other, tx, id, nil, unarchiveWith(nil))
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private archive disclosed: %v", err)
	}
}

func TestDerivedMailAudienceCannotBeReplayedByGenericUndo(t *testing.T) {
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "owner-controlled-mail", "sender@example.test", e.AdminUser)
	decideThread(t, e, e.AdminUser, "owner-controlled-mail", true)
	audit := latestAuditRowID(t, e, entityTypeActivity, id, "update")
	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeActivity, id, audit); answer.Undoable {
		t.Fatal("derived mail audience advertised generic replay")
	}
	if err := undoEntry(t, e, entityTypeActivity, id, audit); err == nil {
		t.Fatal("generic undo replayed the derived audience")
	}
	if got := activityAudience(t, e, id); got != "workspace" {
		t.Fatalf("refused undo changed audience: %s", got)
	}
	decideThread(t, e, e.AdminUser, "owner-controlled-mail", false)
	if got := activityAudience(t, e, id); got != "participants" {
		t.Fatalf("owner's explicit hold did not narrow audience: %s", got)
	}
}
