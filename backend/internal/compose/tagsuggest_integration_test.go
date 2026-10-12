// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The tag scout's model path against a real database. A scripted model reads
// the fenced items out of the prompt as a model would.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// tagBrainStub tags every fenced item whose text contains a cue with the
// cue's tag. reply, when set, replaces the scripted answer; during runs inside
// the call, before it answers.
type tagBrainStub struct {
	cues    map[string]ids.TagID
	reply   func(items []string) (string, error)
	during  func()
	prompts []model.Request
}

func (b *tagBrainStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	b.prompts = append(b.prompts, req)
	spans := fencedSpans(req.System, req.Messages[0].Content)
	if len(spans) == 0 {
		return model.Response{}, fmt.Errorf("the tag prompt fenced no item: %q", req.System)
	}
	if b.during != nil {
		b.during()
	}
	if b.reply != nil {
		ids := make([]string, len(spans))
		for i, s := range spans {
			ids[i] = s.id
		}
		text, err := b.reply(ids)
		return model.Response{Text: text}, err
	}
	answers := make([]tagSuggestAnswer, 0, len(spans))
	for _, s := range spans {
		answer := tagSuggestAnswer{ID: s.id, Tags: []string{}}
		for cue, tag := range b.cues {
			if strings.Contains(s.text, cue) {
				answer.Tags = append(answer.Tags, tag.String())
			}
		}
		answers = append(answers, answer)
	}
	payload, err := json.Marshal(tagSuggestPayload{Items: answers})
	return model.Response{Text: string(payload)}, err
}

// askedAbout lists the item ids the stub was asked about, across calls.
func (b *tagBrainStub) askedAbout() []string {
	var out []string
	for _, req := range b.prompts {
		for _, s := range fencedSpans(req.System, req.Messages[0].Content) {
			out = append(out, s.id)
		}
	}
	return out
}

type fencedSpan struct{ id, text string }

// fencedSpans reads each source_id span out of a prompt, under the marker the
// system prompt declares.
func fencedSpans(system, prompt string) []fencedSpan {
	marker, ok := promptfence.MarkerIn(system)
	if !ok {
		return nil
	}
	var out []fencedSpan
	rest := prompt
	for {
		_, after, found := strings.Cut(rest, "<"+marker+` source_id="`)
		if !found {
			return out
		}
		id, body, _ := strings.Cut(after, `">`)
		text, tail, _ := strings.Cut(body, "</"+marker+">")
		out = append(out, fencedSpan{id: id, text: text})
		rest = tail
	}
}

// suggest runs one model pass as the scout's own principal.
func (e *tagScoutEnv) suggest(t *testing.T, brain completer) (scoutTally, error) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), tagScoutActor)
	return NewTagSuggester(e.Pool, brain, slog.New(slog.DiscardHandler)).RunWorkspace(ctx, clocktest.Now(t))
}

func (e *tagScoutEnv) mustSuggest(t *testing.T, brain completer) scoutTally {
	t.Helper()
	pass, err := e.suggest(t, brain)
	if err != nil {
		t.Fatalf("the tag suggestion pass: %v", err)
	}
	return pass
}

func TestTheModelRaisesASuggestionThePhraseMatchMisses(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	note := e.note(e.Admin(), t, "note", "We still close every month in Excel.", dana, clocktest.Now(t).Add(-time.Hour))

	if pass := e.pass(t, clocktest.Now(t)); pass.Raised != 0 {
		t.Fatalf("the phrase pass raised %d, want 0: the note shares no phrase with the description", pass.Raised)
	}
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	if pass := e.mustSuggest(t, brain); pass.Raised != 1 {
		t.Fatalf("the model pass raised %d suggestions, want 1", pass.Raised)
	}
	open, err := e.tags.OpenTagSuggestions(e.rep(e.Rep1, e.Team1), 10)
	if err != nil || len(open) != 1 || open[0].TagID != migration || open[0].EntityID != dana {
		t.Fatalf("open = %+v (err %v), want Ledger migration on Dana", open, err)
	}
	cited, err := e.tags.GetTagSuggestion(e.rep(e.Rep1, e.Team1), open[0].ID)
	if err != nil || len(cited.Evidence) != 1 || cited.Evidence[0].ActivityID != note {
		t.Fatalf("the suggestion cites %+v (err %v), want the note", cited.Evidence, err)
	}
}

func TestASecondPassDoesNotAskAboutAnItemAlreadyRead(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "Thanks for lunch.", dana, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}

	e.mustSuggest(t, brain)
	if asked := len(brain.askedAbout()); asked != 1 {
		t.Fatalf("the first pass asked about %d items, want 1", asked)
	}
	e.mustSuggest(t, brain)
	if calls := len(brain.prompts); calls != 1 {
		t.Fatalf("the second pass made %d more calls, want none for an item already read", calls-1)
	}
}

func TestEditingASuggestibleTagReadsTheWindowAgain(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	note := e.note(e.Admin(), t, "note", "Wants a named contact for outages.", dana, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"named contact": migration}}
	e.mustSuggest(t, &tagBrainStub{})
	if n := e.WsCount(t, `SELECT count(*) FROM tag_suggestion`); n != 0 {
		t.Fatalf("%d suggestions before the edit, want 0", n)
	}

	description := "wants a named contact when something breaks"
	described := &description
	if _, err := e.tags.UpdateTag(e.Admin(), migration, collections.TagUpdate{Description: &described}, 0); err != nil {
		t.Fatalf("editing the description: %v", err)
	}
	if pass := e.mustSuggest(t, brain); pass.Raised != 1 {
		t.Fatalf("the pass after the edit raised %d, want 1", pass.Raised)
	}
	if asked := brain.askedAbout(); len(asked) != 1 || asked[0] != note.String() {
		t.Fatalf("after the edit the model was asked about %v, want the note again", asked)
	}
}

func TestAReplyNamingAnUnsentItemOrAnUnlistedTagWritesNothing(t *testing.T) {
	for name, reply := range map[string]func(items []string) string{
		"an unlisted tag": func(items []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + ids.NewV7().String() + `"]}]}`
		},
		"an unsent item": func([]string) string {
			return `{"items":[{"id":"` + ids.NewV7().String() + `","tags":[]}]}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupTagScout(t)
			e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
			dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
			e.note(e.Admin(), t, "note", "We close the month in Excel.", dana, clocktest.Now(t).Add(-time.Hour))
			brain := &tagBrainStub{reply: func(items []string) (string, error) { return reply(items), nil }}

			if _, err := e.suggest(t, brain); err == nil {
				t.Fatal("the pass accepted a reply outside what it asked")
			}
			if n := e.WsCount(t, `SELECT (SELECT count(*) FROM tag_suggestion) + (SELECT count(*) FROM tag_interest_read)`); n != 0 {
				t.Fatalf("%d rows written from a refused reply, want 0", n)
			}
		})
	}
}

func TestRestrictedArchivedAndNoiseItemsAreNeverSent(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	at := clocktest.Now(t).Add(-time.Hour)
	kept := e.note(e.Admin(), t, "note", "Excel at month-end.", dana, at)
	archived := e.note(e.Admin(), t, "note", "Excel, archived.", dana, at)
	restricted := e.note(e.Admin(), t, "note", "Excel, restricted.", dana, at)
	noise := e.note(e.Admin(), t, "email", "Excel newsletter.", dana, at)
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	e.WsExec(t, `INSERT INTO activity_retention_evidence
		       (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, restricted)
	e.WsExec(t, `UPDATE activity
		   SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		       restricted_until = now() + interval '1 year',
		       retention_class = 'commercial_correspondence', retention_class_at = now()
		 WHERE id = $1`, restricted)
	e.WsExec(t, `UPDATE activity SET capture_label = 'noise', capture_labeled_at = now() WHERE id = $1`, noise)

	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	e.mustSuggest(t, brain)
	if asked := brain.askedAbout(); len(asked) != 1 || asked[0] != kept.String() {
		t.Fatalf("the model was asked about %v, want only %s", asked, kept)
	}
}

func TestAnItemUnlinkedWhileTheModelReadsItIsNotCited(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	note := e.note(e.Admin(), t, "note", "We close the month in Excel.", dana, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{
		cues:   map[string]ids.TagID{"Excel": migration},
		during: func() { e.WsExec(t, `DELETE FROM activity_link WHERE activity_id = $1`, note) },
	}

	if pass := e.mustSuggest(t, brain); pass.Raised != 0 {
		t.Fatalf("the pass raised %d suggestions on a record the item no longer names", pass.Raised)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag_interest_read WHERE activity_id = $1`, note); n != 1 {
		t.Fatalf("the judged item was stamped %d times, want 1", n)
	}
}

func TestAnOutageEndsThePassCleanlyAndStampsNothing(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "We close the month in Excel.", dana, clocktest.Now(t).Add(-time.Hour))
	down := &ai.ProviderDownError{Provider: "acme", RetryAfter: clocktest.Now(t).Add(time.Hour)}

	if _, err := e.suggest(t, &tagBrainStub{reply: func([]string) (string, error) { return "", down }}); err != nil {
		t.Fatalf("an outage failed the pass (%v), want a clean stop", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag_interest_read`); n != 0 {
		t.Fatalf("%d items stamped read during an outage, want 0", n)
	}
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	if pass := e.mustSuggest(t, brain); pass.Raised != 1 {
		t.Fatalf("the pass after the outage raised %d, want the item read and raised", pass.Raised)
	}
}

func TestOnlyTheOfflineStandInFallsBackToPhrases(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x pricing", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	e.note(e.Admin(), t, "note", "Asked about Product X pricing.", dana, clocktest.Now(t).Add(-time.Hour))
	offline := fmt.Errorf("%w: canned text", ai.ErrUnconfiguredModel)

	pass, err := e.suggest(t, &tagBrainStub{reply: func([]string) (string, error) { return "", offline }})
	if err != nil || pass.Raised != 1 {
		t.Fatalf("with only the offline stand-in the pass raised %d (err %v), want the phrase match", pass.Raised, err)
	}
}
