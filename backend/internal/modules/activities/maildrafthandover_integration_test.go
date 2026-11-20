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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asAdminWith is the other seat as an admin holding grants on activities.
func asAdminWith(e *sendEnv, activity principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
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

// repLeaves deactivates the rep, the state a handover moves drafts from.
func (e *sendEnv) repLeaves(t *testing.T) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.rep); err != nil {
		t.Fatalf("deactivating the rep: %v", err)
	}
}

func handOverDrafts(ctx context.Context, t *testing.T, e *sendEnv) (MailDraftHandover, error) {
	t.Helper()
	var out MailDraftHandover
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		out, err = draftStore(e).HandOverMailDraftsTx(ctx, tx, ids.From[ids.UserKind](e.rep), ids.From[ids.UserKind](e.other))
		return err
	})
	return out, err
}

func (e *sendEnv) draftsOf(t *testing.T, author ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM mail_draft WHERE author_id = $1`, author).Scan(&n); err != nil {
		t.Fatalf("counting drafts: %v", err)
	}
	return n
}

func TestAnAdminCannotReachSomebodyElsesDraft(t *testing.T) {
	e := setupSend(t)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, anchor)
	admin := asAdminWith(e, fullActivityGrant())

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
	e.repLeaves(t)

	out, err := handOverDrafts(asAdminWith(e, fullActivityGrant()), t, e)
	if err != nil || out != (MailDraftHandover{Moved: 1}) {
		t.Fatalf("handing over one draft → %+v (%v), want 1 moved", out, err)
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
	e.repLeaves(t)

	out, err := handOverDrafts(asAdminWith(e, principal.ObjectGrant{Read: true}), t, e)
	if err != nil || out != (MailDraftHandover{}) {
		t.Fatalf("a handover by a caller without activity update → %+v (%v), want nothing moved and no error", out, err)
	}
	if _, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), mine.Anchor); err != nil {
		t.Errorf("the author reading their draft after a refused move → %v, want it still theirs", err)
	}
}

// A member who has not left keeps every draft: an admin handing their work to
// anyone, themselves included, cannot read what they have not sent.
func TestAnActiveAuthorKeepsTheirDraftsThroughAHandover(t *testing.T) {
	e := setupSend(t)
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, replyAnchor(e.seedAnchor(t, "", "")))

	out, err := handOverDrafts(asAdminWith(e, fullActivityGrant()), t, e)
	if err != nil || out != (MailDraftHandover{}) {
		t.Fatalf("a handover of an active author → %+v (%v), want nothing moved", out, err)
	}
	if _, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), mine.Anchor); err != nil {
		t.Errorf("the active author reading their draft after the handover → %v, want it still theirs", err)
	}
}

// A held message names the draft it was composed in and discards it from its
// author when it goes. The draft stays with that author through a handover, so
// the resume takes it and no copy is left with the receiver.
func TestAHeldMessagesDraftStaysForTheResumeToTake(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true}).WithClock(func() time.Time { return draftClock })
	anchor := e.seedAnchor(t, "", "")
	draft := saveDraft(ctx, t, e, replyAnchor(anchor))
	in := soloSendInput("transactional")
	in.MailDraftID = draft.ID
	intent := store.holdForReview(ctx, FromActivity(anchor), in).id
	if intent.IsZero() {
		t.Fatal("the refused message was not held")
	}
	e.repLeaves(t)

	out, err := handOverDrafts(asAdminWith(e, fullActivityGrant()), t, e)
	if err != nil || out != (MailDraftHandover{}) {
		t.Fatalf("a handover over a held message's draft → %+v (%v), want nothing moved", out, err)
	}
	if _, err := store.ResumeHeldSend(ctx, intent, stubConsentGate{}, &deliveryWritingStager{userID: e.rep}); err != nil {
		t.Fatalf("resuming the held message: %v", err)
	}
	if mine, theirs := e.draftsOf(t, e.rep), e.draftsOf(t, e.other); mine != 0 || theirs != 0 {
		t.Errorf("after the resume the author holds %d drafts and the receiver %d, want the draft gone", mine, theirs)
	}
}

// The removal preview asks this count, so a caller who could not move a draft
// sees the blocker before the removal fails.
func TestOnlyALeaversFreeDraftsAreTransferable(t *testing.T) {
	e := setupSend(t)
	saveDraft(e.as(principal.RowScopeAll), t, e, replyAnchor(e.seedAnchor(t, "", "")))
	count := func(ctx context.Context) int {
		t.Helper()
		var n int
		if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			var err error
			n, err = draftStore(e).TransferableMailDraftsTx(ctx, tx, ids.From[ids.UserKind](e.rep))
			return err
		}); err != nil {
			t.Fatalf("counting transferable drafts: %v", err)
		}
		return n
	}
	admin := asAdminWith(e, fullActivityGrant())
	if n := count(admin); n != 0 {
		t.Errorf("an active author's drafts count %d transferable, want 0", n)
	}
	e.repLeaves(t)
	if n := count(asAdminWith(e, principal.ObjectGrant{Read: true})); n != 0 {
		t.Errorf("a caller without activity update counts %d transferable, want 0", n)
	}
	if n := count(admin); n != 1 {
		t.Errorf("a leaver's draft counts %d transferable, want 1", n)
	}
}

// A receiver who saves a draft on the same anchor while the handover runs makes
// the move collide on the one-draft-per-anchor key. The handover answers a
// conflict to retry, never a server error.
func TestAReceiversConcurrentSaveIsAConflict(t *testing.T) {
	e := setupSend(t)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	saveDraft(e.as(principal.RowScopeAll), t, e, anchor)
	e.repLeaves(t)

	ctx := context.Background()
	receiver, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.Exec(ctx, `INSERT INTO mail_draft (id, author_id, anchor_type, anchor_id) VALUES ($1, $2, $3, $4)`,
		ids.NewV7(), e.other, string(anchor.Type), anchor.ID); err != nil {
		t.Fatalf("the receiver's save: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := handOverDrafts(asAdminWith(e, fullActivityGrant()), t, e)
		done <- err
	}()
	e.awaitBlockedOnAnotherTransaction(t)
	if err := receiver.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("the handover racing the receiver's save → %v, want ErrConflict", err)
	}
}

// awaitBlockedOnAnotherTransaction returns once some backend waits on a lock,
// which is the handover's move waiting on the receiver's uncommitted key.
func (e *sendEnv) awaitBlockedOnAnotherTransaction(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, err := e.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Release()
	for {
		var waiting bool
		if err := probe.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE NOT granted)`).Scan(&waiting); err != nil {
			t.Fatalf("the handover never waited on the receiver's save: %v", err)
		}
		if waiting {
			return
		}
	}
}
