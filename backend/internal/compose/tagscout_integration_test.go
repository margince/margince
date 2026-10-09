// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The tag scout against a real database. Tags are coined and marked suggestible
// through the collections store, evidence is logged through the activities
// store, and the Worklist is the production assembly.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type tagScoutEnv struct {
	*integration.Env
	tags *collections.Store
}

func setupTagScout(t *testing.T) *tagScoutEnv {
	t.Helper()
	e := integration.Setup(t)
	return &tagScoutEnv{Env: e, tags: collections.NewStore(e.DB())}
}

// tag coins a word with a description, suggestible or not.
func (e *tagScoutEnv) tag(t *testing.T, name, description string, suggestible bool) ids.TagID {
	t.Helper()
	row, err := e.tags.CreateTag(e.Admin(), name, nil, &description)
	if err != nil {
		t.Fatalf("coining %s: %v", name, err)
	}
	if _, err := e.tags.UpdateTag(e.Admin(), row.ID, collections.TagUpdate{Suggestible: &suggestible}, 0); err != nil {
		t.Fatalf("marking %s suggestible=%v: %v", name, suggestible, err)
	}
	return row.ID
}

// note logs a note on the contact, as the seat in ctx.
func (e *tagScoutEnv) note(ctx context.Context, t *testing.T, kind, body string, contact ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	subject := "Call notes"
	in := activities.LogActivityInput{
		Kind: kind, Subject: &subject, Body: &body, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	if kind == "email" {
		inbound := "inbound"
		in.Direction = &inbound
	}
	logged, _, err := e.Activities.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
	return ids.UUID(logged.Id)
}

// pass runs one scout pass as the scout's own principal.
func (e *tagScoutEnv) pass(t *testing.T, now time.Time) TagScoutPass {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:tag-scout")
	var pass TagScoutPass
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		pass, err = RunTagScout(ctx, tx, now)
		return err
	}); err != nil {
		t.Fatalf("the tag scout pass: %v", err)
	}
	return pass
}

func (e *tagScoutEnv) rep(user, team ids.UUID) context.Context {
	return e.As(user, []ids.UUID{team}, integration.AccountRepPerms)
}

// worklistTagSuggestions answers the tag suggestion ids the reader's needs_you
// lane carries, from the production assembly. The lane also reads approvals, so
// the reader holds the admin grid.
func (e *tagScoutEnv) worklistTagSuggestions(ctx context.Context, t *testing.T) []string {
	t.Helper()
	day, err := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now).Assemble(ctx)
	if err != nil {
		t.Fatalf("assembling the day: %v", err)
	}
	var out []string
	for _, item := range day.NeedsYou {
		if item.Source == "tag_suggestion" {
			out = append(out, item.Id)
		}
	}
	return out
}

func TestASuggestibleTagIsProposedOnTheWorklistAndAcceptedAsTheHuman(t *testing.T) {
	e := setupTagScout(t)
	productX := e.tag(t, "Product X", "demo of product x, product x pricing", true)
	e.tag(t, "Webinar", "demo", false)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "Dana asked for a Demo of Product X next week.", dana, time.Now().Add(-time.Hour))

	if pass := e.pass(t, time.Now()); pass.Raised != 1 {
		t.Fatalf("the pass raised %d suggestions, want 1 for the suggestible tag alone", pass.Raised)
	}
	rep1 := e.rep(e.Rep1, e.Team1)
	shown := e.worklistTagSuggestions(e.Admin(), t)
	if len(shown) != 1 {
		t.Fatalf("the Worklist carries %d tag suggestions, want 1", len(shown))
	}
	id := ids.MustParse(shown[0])
	suggestion, err := e.tags.GetTagSuggestion(rep1, id)
	if err != nil || suggestion.TagID != productX || suggestion.EntityID != dana || len(suggestion.Evidence) != 1 {
		t.Fatalf("suggestion = %+v (err %v), want Product X on Dana citing the note", suggestion, err)
	}

	if _, err := e.tags.AcceptTagSuggestion(rep1, id); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if by := e.WsScalar(t, `SELECT assigned_by::text || '/' || assigned_by_kind FROM taggable
		WHERE tag_id = $1 AND entity_type = 'contact' AND entity_id = $2`, productX, dana); by != e.Rep1.String()+"/human" {
		t.Fatalf("the tagging was assigned by %q, want the accepting rep as a human", by)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'tag' AND entity_id = $1
		AND evidence->>'tag_suggestion_id' = $2`, productX, id.String()); n != 1 {
		t.Fatalf("%d tag audit rows name the suggestion, want 1", n)
	}
	if pass := e.pass(t, time.Now()); pass.Raised != 0 {
		t.Fatalf("a record carrying the tag was offered it again (%d raised)", pass.Raised)
	}
}

func TestADismissedTagSuggestionReturnsOnlyOnNewerEvidence(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x pricing", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "Asked about Product X pricing.", dana, time.Now().Add(-2*time.Hour))
	e.pass(t, time.Now())
	rep1 := e.rep(e.Rep1, e.Team1)
	open, err := e.tags.OpenTagSuggestions(rep1, 10)
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %d (err %v), want 1", len(open), err)
	}

	if _, err := e.tags.DismissTagSuggestion(rep1, open[0].ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if pass := e.pass(t, time.Now()); pass.Raised != 0 {
		t.Fatalf("the dismissed evidence raised %d suggestions again", pass.Raised)
	}

	fresh := e.note(e.Admin(), t, "meeting", "Wants Product X pricing for 40 seats.", dana, time.Now())
	if pass := e.pass(t, time.Now().Add(time.Minute)); pass.Raised != 1 {
		t.Fatalf("newer evidence raised %d suggestions, want 1", pass.Raised)
	}
	again, err := e.tags.OpenTagSuggestions(rep1, 10)
	if err != nil || len(again) != 1 {
		t.Fatalf("open after new evidence = %d (err %v), want 1", len(again), err)
	}
	cited, err := e.tags.GetTagSuggestion(rep1, again[0].ID)
	if err != nil || len(cited.Evidence) != 1 || cited.Evidence[0].ActivityID != fresh {
		t.Fatalf("the new suggestion cites %+v (err %v), want only the newer meeting", cited.Evidence, err)
	}
}

func TestATagSuggestionFromOwnerOnlyMailIsShownOnlyToItsOwner(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x demo", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	rep1, rep3 := e.rep(e.Rep1, e.Team1), e.rep(e.Rep3, e.Team2)
	mail := e.note(rep1, t, "email", "Could we get a Product X demo?", dana, time.Now().Add(-time.Hour))
	if _, err := e.Activities.SetAudience(rep1, ids.From[ids.ActivityKind](mail),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail to its owner: %v", err)
	}
	e.pass(t, time.Now())

	mine, err := e.tags.OpenTagSuggestions(rep1, 10)
	if err != nil || len(mine) != 1 {
		t.Fatalf("the mail's owner is shown %d suggestions (err %v), want 1", len(mine), err)
	}
	if shown := e.worklistTagSuggestions(e.Admin(), t); len(shown) != 0 {
		t.Fatalf("an admin's Worklist carries %d tag suggestions built from mail they may not read", len(shown))
	}
	if n, err := e.tags.CountOpenTagSuggestions(rep3); err != nil || n != 0 {
		t.Fatalf("a colleague counts %d (err %v), want 0", n, err)
	}
	if _, err := e.tags.DismissTagSuggestion(rep3, mine[0].ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a colleague dismissing it = %v, want not found", err)
	}
}

func TestATagSuggestionsAuditRowAndEventNameNeitherTheTagNorTheRecord(t *testing.T) {
	e := setupTagScout(t)
	productX := e.tag(t, "Product X", "product x demo", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	rep1 := e.rep(e.Rep1, e.Team1)
	mail := e.note(rep1, t, "email", "Could we get a Product X demo?", dana, time.Now().Add(-time.Hour))
	if _, err := e.Activities.SetAudience(rep1, ids.From[ids.ActivityKind](mail),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail to its owner: %v", err)
	}
	e.pass(t, time.Now())

	// The admin reads the compliance log and is outside the mail's audience.
	entityType := "tag_suggestion"
	page, err := privacy.ListAuditLog(e.Admin(), e.DB(), privacy.AuditFilter{EntityType: &entityType})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("the audit log holds %d tag suggestion rows (err %v), want 1", len(page.Entries), err)
	}
	for _, leak := range []string{productX.String(), dana.String()} {
		if strings.Contains(string(page.Entries[0].After), leak) {
			t.Fatalf("the audit image %s names %s", page.Entries[0].After, leak)
		}
	}
	envelope := e.WsScalar(t, `SELECT envelope::text FROM event_outbox WHERE envelope->>'type' = 'tag_suggestion.created'`)
	if strings.Contains(envelope, dana.String()) || strings.Contains(envelope, productX.String()) {
		t.Fatalf("the created event %s names the tag or the record", envelope)
	}
}

func TestADecidedTagSuggestionIsGoneFromReadsAndConflictsOnlyForAWriter(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x pricing", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "Asked about Product X pricing.", dana, time.Now().Add(-time.Hour))
	e.pass(t, time.Now())
	rep1, rep3 := e.rep(e.Rep1, e.Team1), e.rep(e.Rep3, e.Team2)
	open, err := e.tags.OpenTagSuggestions(rep1, 10)
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %d (err %v), want 1", len(open), err)
	}
	id := open[0].ID
	if _, err := e.tags.AcceptTagSuggestion(rep1, id); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'tag_suggestion.accepted'`); n != 1 {
		t.Fatalf("%d accepted events, want 1", n)
	}

	if _, err := e.tags.GetTagSuggestion(rep1, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a decided suggestion = %v, want not found", err)
	}
	var decided *collections.TagSuggestionDecidedError
	if _, err := e.tags.DismissTagSuggestion(rep1, id); !errors.As(err, &decided) {
		t.Fatalf("the record's writer dismissing it again = %v, want suggestion_decided", err)
	}
	if _, err := e.tags.DismissTagSuggestion(rep3, id); err == nil || errors.As(err, &decided) {
		t.Fatalf("a colleague who may not write the record = %v, want a refusal that is not a conflict", err)
	}
	if _, err := e.Contacts.ArchiveContact(e.Admin(), ids.From[ids.ContactKind](dana), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := e.tags.DismissTagSuggestion(rep1, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deciding on an archived record = %v, want not found", err)
	}
}

func TestATagSuggestionCitingRestrictedMailIsRetiredAndFreesTheRecord(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x pricing", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	held := e.note(e.Admin(), t, "note", "Asked about Product X pricing.", dana, time.Now().Add(-time.Hour))
	e.pass(t, time.Now())
	// The whole shape a restriction takes; the database refuses a partial one.
	e.WsExec(t, `INSERT INTO activity_retention_evidence
		       (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, held)
	e.WsExec(t, `UPDATE activity
		   SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		       restricted_until = now() + interval '1 year',
		       retention_class = 'commercial_correspondence', retention_class_at = now()
		 WHERE id = $1`, held)

	if pass := e.pass(t, time.Now()); pass.Superseded != 1 {
		t.Fatalf("the pass retired %d suggestions, want the one citing restricted mail", pass.Superseded)
	}
	e.note(e.Admin(), t, "meeting", "Wants Product X pricing for 40 seats.", dana, time.Now())
	if pass := e.pass(t, time.Now().Add(time.Minute)); pass.Raised != 1 {
		t.Fatalf("new evidence raised %d suggestions, want 1", pass.Raised)
	}
}
