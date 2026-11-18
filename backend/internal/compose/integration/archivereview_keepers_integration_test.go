// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// logArchivedMail logs an outbound mail through the activity store, linked to
// the contact, and archives it a month ago.
func logArchivedMail(t *testing.T, e *Env, subject string, occurred time.Time, contact ids.UUID) ids.UUID {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new(subject), Body: new("What was said."), Direction: new("outbound"),
		OccurredAt: &occurred, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	id := ids.UUID(logged.Id)
	e.WsExec(t, `UPDATE activity SET archived_at = $2 WHERE id = $1`, id, clocktest.Now(t).AddDate(0, -1, 0))
	return id
}

// deleteArchived posts one deletion request as ctx and decodes the answer.
func deleteArchived(ctx context.Context, t *testing.T, e *Env, named ...ids.UUID) crmcontracts.ArchiveDeletionResult {
	t.Helper()
	quoted := make([]string, 0, len(named))
	for _, id := range named {
		quoted = append(quoted, `"`+id.String()+`"`)
	}
	rec := httptest.NewRecorder()
	compose.NewPrivacyHandlers(e.Pool).DeleteArchivedActivities(rec, httptest.NewRequest(http.MethodPost,
		"/v1/retention/archive/delete", strings.NewReader(`{"activity_ids":[`+strings.Join(quoted, ",")+`]}`)).
		WithContext(ctx), crmcontracts.DeleteArchivedActivitiesParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	var out crmcontracts.ArchiveDeletionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func keptReasons(out crmcontracts.ArchiveDeletionResult) map[ids.UUID]crmcontracts.ArchiveKeptReason {
	kept := map[ids.UUID]crmcontracts.ArchiveKeptReason{}
	for _, k := range out.Kept {
		kept[ids.UUID(k.ActivityId)] = k.Reason
	}
	return kept
}

// TestTheArchiveDeletionKeepsWhatTheLawKeeps deletes three archived mails of a
// year ago. One is linked to a won deal, so the statutory floor keeps it. One
// a controller pinned to the floor. Only the third, which nothing keeps, is
// erased.
func TestTheArchiveDeletionKeepsWhatTheLawKeeps(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	yearAgo := clocktest.Now(t).AddDate(-1, 0, 0)
	contact := e.SeedContact(t, "Floor Subject", nil)
	won := logArchivedMail(t, e, "Signed order", yearAgo, contact)
	e.SeedWonDealLinkedTo(t, won)
	pinned := logArchivedMail(t, e, "Supplier letter", yearAgo, contact)
	reason, err := privacy.ParseStatedReason("supplier correspondence: §257 HGB, no deal in the CRM")
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.ComposedEraser(e.Pool, nil, nil).PinToFloor(
		controllerCtx(e, principal.ObjectGrant{Read: true, Update: true}), pinned, reason); err != nil {
		t.Fatalf("pinning the supplier letter: %v", err)
	}
	free := logArchivedMail(t, e, "Old greeting", yearAgo, contact)

	out := deleteArchived(archiveReviewerCtx(e, principal.ObjectGrant{Read: true, Delete: true}), t, e, won, pinned, free)
	if len(out.Deleted) != 1 || ids.UUID(out.Deleted[0]) != free {
		t.Errorf("deleted %v, want only the mail nothing keeps (%s)", out.Deleted, free)
	}
	kept := keptReasons(out)
	for name, id := range map[string]ids.UUID{"the won deal's mail": won, "the pinned letter": pinned} {
		if kept[id] != crmcontracts.ArchiveKeptReasonKeptByLaw {
			t.Errorf("%s kept as %q, want kept_by_law (all kept: %v)", name, kept[id], kept)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND body = 'What was said.'`,
		[]ids.UUID{won, pinned}); n != 2 {
		t.Errorf("%d of the two records the law keeps still hold their content, want 2", n)
	}
}

// TestAnOptOutReceiptPastItsFloorIsDeleted archives the receipt a refusal
// produced and lets its floor run out. The receipt no longer keeps it, so
// the controller's deletion erases it.
func TestAnOptOutReceiptPastItsFloorIsDeleted(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	refuser := l.refusingContact(t, "Lapsed Refusal")
	var receipt ids.UUID
	owner := OwnerConn(t)
	if err := owner.QueryRow(l.ctx, `SELECT o.activity_id FROM comms_outbound o
		JOIN activity_link k ON k.activity_id = o.activity_id
		WHERE o.template_key = 'optout_acknowledgement' AND k.contact_id = $1`, refuser).Scan(&receipt); err != nil {
		t.Fatalf("finding the receipt: %v", err)
	}
	now := clocktest.Now(t)
	if _, err := owner.Exec(l.ctx, `UPDATE comms_outbound SET kept_until = $2 WHERE activity_id = $1`,
		receipt, now.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(l.ctx, `UPDATE activity SET archived_at = $2 WHERE id = $1`, receipt, now); err != nil {
		t.Fatal(err)
	}

	out := deleteArchived(archiveReviewerCtx(l.e, principal.ObjectGrant{Read: true, Delete: true}), t, l.e, receipt)
	if len(out.Deleted) != 1 || ids.UUID(out.Deleted[0]) != receipt {
		t.Errorf("deleted %v and kept %v, want the lapsed receipt %s deleted", out.Deleted, out.Kept, receipt)
	}
}

// TestTheArchiveReviewHidesMailOnlyItsCapturerMaySee reviews the archive as a
// colleague, once with an own row scope and once as an all-scope admin. The
// mail linked only to another user's unpromoted captured contact is neither
// listed nor deleted, and reads as not found. The mail linked to a workspace
// contact is listed and deleted.
func TestTheArchiveReviewHidesMailOnlyItsCapturerMaySee(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	yearAgo := clocktest.Now(t).AddDate(-1, 0, 0)
	inside := logArchivedMail(t, e, "Workspace customer", yearAgo, e.SeedContact(t, "Workspace Subject", &e.Rep1))
	captured := e.SeedContact(t, "Captured Subject", &e.Rep3)
	outside := logArchivedMail(t, e, "Unpromoted capture", yearAgo, captured)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, captured)
	reviewer := func(scope principal.RowScope) context.Context {
		return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{RowScope: scope, Objects: map[string]principal.ObjectGrant{
			"retention_policy": {Read: true, Delete: true}, objContact: {Read: true}, "activity": {Read: true},
		}})
	}
	h := compose.NewPrivacyHandlers(e.Pool)
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeAll} {
		as := httptest.NewRequest(http.MethodGet, "/v1/retention/archive", nil).WithContext(reviewer(scope))
		_, rows := listArchive(t, h, as, crmcontracts.ListArchivedActivitiesParams{})
		listed := make([]ids.UUID, 0, len(rows))
		for _, row := range rows {
			listed = append(listed, ids.UUID(row.ActivityId))
		}
		if !slices.Contains(listed, inside) || slices.Contains(listed, outside) {
			t.Errorf("row scope %s listed %v, want %s and not %s", scope, listed, inside, outside)
		}
		out := deleteArchived(reviewer(scope), t, e, outside)
		if reason := keptReasons(out)[outside]; len(out.Deleted) != 0 || reason != crmcontracts.ArchiveKeptReasonNotFound {
			t.Errorf("row scope %s deleting the hidden mail: deleted %v, kept as %q, want not_found", scope, out.Deleted, reason)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND body = 'What was said.'`, outside); n != 1 {
		t.Error("a reviewer erased mail only its capturer may see")
	}
	if out := deleteArchived(reviewer(principal.RowScopeOwn), t, e, inside); len(out.Deleted) != 1 {
		t.Errorf("the workspace contact's archived mail was not deleted: kept %v", out.Kept)
	}
}
