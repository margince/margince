// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asOtherWith is the other seat holding grants on activities, in every row.
func asOtherWith(e *sendEnv, role string, activity principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other,
		Permissions: principal.Permissions{
			RoleKeys: []string{role},
			Objects: map[string]principal.ObjectGrant{
				"activity": activity, "contact": {Read: true, Update: true}, "user_admin": {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func fullActivityGrant() principal.ObjectGrant {
	return principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
}

func handOverDrafts(ctx context.Context, t *testing.T, e *sendEnv) (int, error) {
	t.Helper()
	var moved int
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		moved, err = draftStore(e).HandOverMailDraftsTx(ctx, tx, ids.From[ids.UserKind](e.rep), ids.From[ids.UserKind](e.other))
		return err
	})
	return moved, err
}

func TestAnAdminCannotReachSomebodyElsesDraft(t *testing.T) {
	e := setupSend(t)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, anchor)
	admin := asOtherWith(e, "admin", fullActivityGrant())

	if _, err := draftStore(e).GetMailDraft(admin, anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an admin reading my draft → %v, want ErrNotFound", err)
	}
	if err := draftStore(e).DiscardMailDraft(admin, mine.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an admin discarding my draft → %v, want ErrNotFound", err)
	}
	if !e.draftExists(t, mine.ID) {
		t.Fatal("my draft is gone after an admin's attempts")
	}
}

// The moved draft is audited as an update naming its new author, and like
// every draft audit row it carries no word the author wrote.
func TestAHandedOverDraftBelongsToTheSuccessorAndIsAudited(t *testing.T) {
	e := setupSend(t)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, anchor)

	moved, err := handOverDrafts(asOtherWith(e, "admin", fullActivityGrant()), t, e)
	if err != nil || moved != 1 {
		t.Fatalf("handing over one draft → %d (%v), want 1", moved, err)
	}
	got, err := draftStore(e).GetMailDraft(asColleague(e), anchor)
	if err != nil || got.ID != mine.ID || got.Content.Body != typedDraft().Body || got.Version != mine.Version+1 {
		t.Fatalf("the successor reading the draft → %v v%d %q (%v), want the same draft one version on", got.ID, got.Version, got.Content.Body, err)
	}
	if _, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the former author reading the draft → %v, want ErrNotFound", err)
	}
	actions, images := e.draftTrail(t, mine.ID)
	if !slices.Equal(actions, []string{"create", "update"}) || !strings.Contains(images, e.other.String()) {
		t.Errorf("trail %v %s, want create then an update naming the successor", actions, images)
	}
	if strings.Contains(images, typedDraft().Body) {
		t.Errorf("the trail carries the draft's words: %s", images)
	}
}

// A caller who may not change activities moves no draft, and the rest of the
// handover still lands, as for open tasks.
func TestAHandoverWithoutTheActivityUpdateGrantMovesNoDraft(t *testing.T) {
	e := setupSend(t)
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, replyAnchor(e.seedAnchor(t, "", "")))

	moved, err := handOverDrafts(asOtherWith(e, "admin", principal.ObjectGrant{Read: true}), t, e)
	if err != nil || moved != 0 {
		t.Fatalf("a handover by a caller without activity update → %d (%v), want 0 and no error", moved, err)
	}
	if _, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), mine.Anchor); err != nil {
		t.Errorf("the author reading their draft after a refused move → %v, want it still theirs", err)
	}
}
