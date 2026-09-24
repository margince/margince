// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A rep's saved draft, against a real database: that it reads back to its
// author and nobody else, that two composers cannot overwrite each other, and
// that the transaction which sends or schedules a message takes the draft
// with it while a refusal leaves it where it was.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var draftClock = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// recordingTimer stands where the job runner is: the one boundary the
// schedule path crosses that this package does not own.
type recordingTimer struct{ due []time.Time }

func (r *recordingTimer) ScheduleTx(_ context.Context, _ pgx.Tx, _ ids.UUID, due time.Time) error {
	r.due = append(r.due, due)
	return nil
}

func draftStore(e *sendEnv) *Store {
	return e.store(stubUnsubscribeLinker{}).WithClock(func() time.Time { return draftClock })
}

func replyAnchor(id ids.ActivityID) MailDraftAnchor {
	return MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeActivity, ID: id.UUID}
}

func typedDraft() MailDraftContent {
	return MailDraftContent{
		To:       []string{"buyer@example.test"},
		Bcc:      []string{"bu"},
		Subject:  "Re: pricing",
		Body:     "Half a thought",
		HTMLBody: "<p>Half a thought</p>",
	}
}

// saveDraft seeds through the real writer, the way the composer does.
func saveDraft(ctx context.Context, t *testing.T, e *sendEnv, anchor MailDraftAnchor) MailDraft {
	t.Helper()
	saved, err := draftStore(e).SaveMailDraft(ctx, anchor, typedDraft(), nil)
	if err != nil {
		t.Fatalf("saving the draft: %v", err)
	}
	return saved
}

func (e *sendEnv) draftExists(t *testing.T, id ids.UUID) bool {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM mail_draft WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("counting drafts: %v", err)
	}
	return n == 1
}

// draftTrail reads the audit actions recorded against one draft, oldest
// first, and the images they carry.
func (e *sendEnv) draftTrail(t *testing.T, id ids.UUID) (actions []string, images string) {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT action, coalesce(before::text, '') || coalesce(after::text, '')
		  FROM audit_log WHERE entity_type = 'mail_draft' AND entity_id = $1
		 ORDER BY occurred_at, id`, id)
	if err != nil {
		t.Fatalf("reading the draft's audit trail: %v", err)
	}
	defer rows.Close()
	var all strings.Builder
	for rows.Next() {
		var action, image string
		if err := rows.Scan(&action, &image); err != nil {
			t.Fatalf("scanning the draft's audit trail: %v", err)
		}
		actions = append(actions, action)
		all.WriteString(image)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the draft's audit trail: %v", err)
	}
	return actions, all.String()
}

func TestASavedDraftReadsBackToItsAuthorAsTyped(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))

	saved := saveDraft(ctx, t, e, anchor)
	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatalf("reading the draft back: %v", err)
	}

	want := typedDraft()
	if got.ID != saved.ID || got.Version != 1 {
		t.Errorf("read back draft %s v%d, want %s v1", got.ID, got.Version, saved.ID)
	}
	if got.Content.Subject != want.Subject || got.Content.Body != want.Body || got.Content.HTMLBody != want.HTMLBody {
		t.Errorf("read back %+v, want what was typed %+v", got.Content, want)
	}
	if len(got.Content.Bcc) != 1 || got.Content.Bcc[0] != "bu" {
		t.Errorf("a half-typed address came back as %v — a draft keeps what the rep had, valid or not", got.Content.Bcc)
	}
	if len(got.Content.Cc) != 0 {
		t.Errorf("an untouched cc line came back as %v, want empty", got.Content.Cc)
	}
}

func TestNoDraftForAnAnchorIsNotFound(t *testing.T) {
	e := setupSend(t)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}

	_, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), anchor)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a draft nobody saved → %v, want ErrNotFound", err)
	}
}

// Replacing needs the version the composer last read; a stale one, or a
// second "create" where a draft already exists, is refused rather than
// silently overwriting another composer's text.
func TestADraftIsReplacedOnlyAtTheVersionTheComposerRead(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	saved := saveDraft(ctx, t, e, anchor)
	store := draftStore(e)

	edited := typedDraft()
	edited.Body = "The whole thought"
	replaced, err := store.SaveMailDraft(ctx, anchor, edited, &saved.Version)
	if err != nil {
		t.Fatalf("replacing at the read version: %v", err)
	}
	if replaced.ID != saved.ID || replaced.Version != 2 || replaced.Content.Body != edited.Body {
		t.Fatalf("replaced draft = %s v%d %q, want %s v2 %q", replaced.ID, replaced.Version, replaced.Content.Body, saved.ID, edited.Body)
	}

	if _, err := store.SaveMailDraft(ctx, anchor, typedDraft(), &saved.Version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("saving over a stale version → %v, want ErrVersionSkew", err)
	}
	if _, err := store.SaveMailDraft(ctx, anchor, typedDraft(), nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("creating where a draft already exists → %v, want ErrVersionSkew", err)
	}
	got, err := store.GetMailDraft(ctx, anchor)
	if err != nil || got.Content.Body != edited.Body {
		t.Fatalf("after two refused saves the draft reads %q (%v), want the accepted edit %q", got.Content.Body, err, edited.Body)
	}
}

// A draft is its author's alone: a colleague with the same grants and scope
// can neither read it, overwrite it, nor discard it, and learns only "not
// found" — the answer a missing draft gives.
func TestAColleagueCannotReachSomebodyElsesDraft(t *testing.T) {
	e := setupSend(t)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	mine := saveDraft(e.as(principal.RowScopeAll), t, e, anchor)
	colleague := asColleague(e)
	store := draftStore(e)

	if _, err := store.GetMailDraft(colleague, anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague reading my draft → %v, want ErrNotFound", err)
	}
	if err := store.DiscardMailDraft(colleague, mine.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague discarding my draft → %v, want ErrNotFound", err)
	}
	if _, err := store.SaveMailDraft(colleague, anchor, MailDraftContent{Body: "mine now"}, &mine.Version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("a colleague saving at my draft's version → %v, want ErrVersionSkew on a draft of their own they do not have", err)
	}

	got, err := store.GetMailDraft(e.as(principal.RowScopeAll), anchor)
	if err != nil || got.Version != mine.Version || got.Content.Body != typedDraft().Body {
		t.Fatalf("my draft after a colleague's attempts reads v%d %q (%v), want it untouched", got.Version, got.Content.Body, err)
	}
}

// Saving is a read of the anchor, and so is reading a draft back: a record the
// author cannot see answers not found, whether it always was out of reach or
// fell out of reach after the draft was written.
func TestADraftAnswersToItsAnchorsRowScope(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeOwn)
	hidden := replyAnchor(e.seedAnchor(t, "", ""))
	e.linkToContactOwnedBy(t, ids.From[ids.ActivityKind](hidden.ID), e.other)

	if _, err := draftStore(e).SaveMailDraft(ctx, hidden, typedDraft(), nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("saving against an anchor outside the caller's scope → %v, want ErrNotFound", err)
	}

	visible := e.seedAnchor(t, "", "")
	saveDraft(ctx, t, e, replyAnchor(visible))
	e.linkToContactOwnedBy(t, visible, e.other)
	if _, err := draftStore(e).GetMailDraft(ctx, replyAnchor(visible)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a draft whose anchor fell out of scope → %v, want ErrNotFound", err)
	}
}

// Every save and discard lands in the audit trail, and none of them carries a
// word the rep wrote: the compliance log learns a draft existed, not what it
// said.
func TestADraftsTrailRecordsEachActWithoutItsContent(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	saved := saveDraft(ctx, t, e, anchor)
	store := draftStore(e)
	if _, err := store.SaveMailDraft(ctx, anchor, typedDraft(), &saved.Version); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if err := store.DiscardMailDraft(ctx, saved.ID); err != nil {
		t.Fatalf("discarding: %v", err)
	}

	actions, images := e.draftTrail(t, saved.ID)
	if strings.Join(actions, ",") != "create,update,delete" {
		t.Errorf("audit trail = %v, want create, update, delete", actions)
	}
	for _, word := range []string{"buyer@example.test", "Re: pricing", "Half a thought"} {
		if strings.Contains(images, word) {
			t.Errorf("the draft's audit images quote %q; they carry the anchor and version only", word)
		}
	}
	if e.draftExists(t, saved.ID) {
		t.Error("a discarded draft is still in the table")
	}
}

func TestSendingARepliesDraftDiscardsItInTheSameTransaction(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	draft := saveDraft(ctx, t, e, replyAnchor(anchor))
	in := sendInput("transactional")
	in.MailDraftID = draft.ID

	if _, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), in, stubConsentGate{}, &recordingStager{}); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if e.draftExists(t, draft.ID) {
		t.Error("the draft a sent message was composed in outlived the send")
	}
	if actions, _ := e.draftTrail(t, draft.ID); len(actions) == 0 || actions[len(actions)-1] != "delete" {
		t.Errorf("the send left no discard in the draft's trail: %v", actions)
	}
}

// The draft goes with a COMMITTED send only. A send that fails unwinds, and
// the rep is still in the composer with what they wrote.
func TestARefusedSendKeepsTheDraft(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	draft := saveDraft(ctx, t, e, replyAnchor(anchor))
	in := sendInput("transactional")
	in.MailDraftID = draft.ID

	_, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), in, stubConsentGate{},
		&recordingStager{err: errors.New("delivery table unavailable")})
	if err == nil {
		t.Fatal("SendEmail reported success though staging refused")
	}
	if !e.draftExists(t, draft.ID) {
		t.Error("a send that committed nothing still discarded the draft")
	}
}

// A consent refusal freezes the message for review and answers the rep with
// the refusal. They are still in the composer, so the draft stays too.
func TestAConsentRefusalHeldForReviewKeepsTheDraft(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	draft := saveDraft(ctx, t, e, replyAnchor(anchor))
	in := sendInput("transactional")
	in.MailDraftID = draft.ID

	// The engine answers at staging, which is where the real refusal arrives.
	_, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), in,
		stubConsentGate{}, &recordingStager{err: apperrors.ErrConsentNotGranted})
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("SendEmail to an unconsented recipient → %v, want ErrConsentNotGranted", err)
	}
	var held int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil || held != 1 {
		t.Fatalf("held messages = %d (%v), want the refused one frozen for review", held, err)
	}
	if !e.draftExists(t, draft.ID) {
		t.Error("a message refused and held for review discarded the draft the rep is still looking at")
	}
}

func TestSchedulingADraftDiscardsItInTheSameTransaction(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	draft := saveDraft(ctx, t, e, replyAnchor(anchor))
	in := sendInput("transactional")
	in.MailDraftID = draft.ID
	timer := &recordingTimer{}

	out, err := draftStore(e).SendOrSchedule(ctx, FromActivity(anchor), in,
		&SendSchedule{At: draftClock.Add(24 * time.Hour), TZ: "Europe/Berlin"},
		stubConsentGate{}, &recordingStager{}, timer)
	if err != nil {
		t.Fatalf("SendOrSchedule: %v", err)
	}
	if out.Scheduled == nil || len(timer.due) != 1 {
		t.Fatalf("scheduling produced %+v with %d timers, want one scheduled message", out, len(timer.due))
	}
	if e.draftExists(t, draft.ID) {
		t.Error("the draft a scheduled message was composed in outlived the scheduling")
	}
}

// Only the draft for THIS message's anchor goes. A draft id from another
// place, or one named by an agent acting for the rep, is ignored rather than
// refused — a stale id must never stop a message — and the draft survives.
func TestASendDiscardsOnlyItsOwnAnchorsDraftAndOnlyForAHuman(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	elsewhere := saveDraft(ctx, t, e, MailDraftAnchor{
		Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Somebody else"),
	})
	in := sendInput("transactional")
	in.MailDraftID = elsewhere.ID
	if _, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), in, stubConsentGate{}, &recordingStager{}); err != nil {
		t.Fatalf("SendEmail naming another anchor's draft: %v", err)
	}
	if !e.draftExists(t, elsewhere.ID) {
		t.Error("a reply discarded a draft written on a different record")
	}

	own := saveDraft(ctx, t, e, replyAnchor(anchor))
	agent := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:helper", UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := discardInOwnTx(agent, t, e, own.ID, FromActivity(anchor)); err != nil {
		t.Fatalf("discarding as an agent: %v", err)
	}
	if !e.draftExists(t, own.ID) {
		t.Error("an agent acting for the rep discarded the rep's draft")
	}
}

func discardInOwnTx(ctx context.Context, t *testing.T, e *sendEnv, id ids.UUID, origin SendOrigin) error {
	t.Helper()
	return draftStore(e).tx(ctx, func(tx pgx.Tx) error {
		return discardComposedDraftTx(ctx, tx, id, origin)
	})
}

// Addresses are kept the way the send path compares them, so the erasure and
// the subject export find a draft by the address they hold.
func TestADraftKeepsItsAddressesTrimmedAndLowercased(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	content := typedDraft()
	content.To = []string{"  Buyer@Example.TEST ", "   "}

	saved, err := draftStore(e).SaveMailDraft(ctx, anchor, content, nil)
	if err != nil {
		t.Fatalf("saving: %v", err)
	}
	if len(saved.Content.To) != 1 || saved.Content.To[0] != "buyer@example.test" {
		t.Fatalf("To line saved as %q, want the one address trimmed and lowercased", saved.Content.To)
	}
}

// A draft nobody saves again for the retention window is deleted, whatever
// became of its anchor; a newer one stays. Only the system sweeps.
func TestTheRetentionSweepDeletesOnlyDraftsPastTheWindow(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	stale := saveDraft(ctx, t, e, replyAnchor(e.seedAnchor(t, "", "")))
	fresh := saveDraft(ctx, t, e, replyAnchor(e.seedAnchor(t, "", "")))
	for id, age := range map[ids.UUID]time.Duration{
		stale.ID: MailDraftRetention + time.Hour, fresh.ID: MailDraftRetention - time.Hour,
	} {
		if _, err := e.owner.Exec(context.Background(),
			`UPDATE mail_draft SET updated_at = $2 WHERE id = $1`, id, draftClock.Add(-age)); err != nil {
			t.Fatalf("ageing a draft: %v", err)
		}
	}

	if _, err := draftStore(e).PurgeStaleMailDrafts(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep running the sweep → %v, want ErrPermissionDenied", err)
	}
	system := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	purged, err := draftStore(e).PurgeStaleMailDrafts(system)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if purged != 1 || e.draftExists(t, stale.ID) || !e.draftExists(t, fresh.ID) {
		t.Fatalf("purged %d; stale kept=%v, fresh kept=%v — want only the stale one gone",
			purged, e.draftExists(t, stale.ID), e.draftExists(t, fresh.ID))
	}
	if actions, _ := e.draftTrail(t, stale.ID); len(actions) == 0 || actions[len(actions)-1] != "delete" {
		t.Errorf("the sweep left no delete in the stale draft's trail: %v", actions)
	}
}
