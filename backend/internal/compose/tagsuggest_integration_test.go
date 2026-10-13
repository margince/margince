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

	"github.com/margince/margince/backend/internal/modules/activities"
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

	if pass := e.mustSuggest(t, &tagBrainStub{}); pass.Raised != 0 {
		t.Fatalf("a bound model that found nothing raised %d; the phrase pass must not stand in for it", pass.Raised)
	}
	e.WsExec(t, `DELETE FROM tag_interest_read`)
	offline := fmt.Errorf("%w: canned text", ai.ErrUnconfiguredModel)
	pass, err := e.suggest(t, &tagBrainStub{reply: func([]string) (string, error) { return "", offline }})
	if err != nil || pass.Raised != 1 {
		t.Fatalf("with only the offline stand-in the pass raised %d (err %v), want the phrase match", pass.Raised, err)
	}
}

func TestAnItemEditedAfterItWasReadIsReadAgain(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	meeting := e.note(e.Admin(), t, "meeting", "Agenda to follow.", dana, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	if pass := e.mustSuggest(t, brain); pass.Raised != 0 {
		t.Fatalf("the empty meeting raised %d", pass.Raised)
	}

	e.WsExec(t, `UPDATE activity SET body = 'They close the month in Excel and want out.' WHERE id = $1`, meeting)
	if pass := e.mustSuggest(t, brain); pass.Raised != 1 {
		t.Fatalf("the edited meeting raised %d, want 1", pass.Raised)
	}
	if asked := brain.askedAbout(); len(asked) != 2 {
		t.Fatalf("the model was asked about %v, want the meeting before and after its edit", asked)
	}
}

func TestAnAnswerRefusedByAnOpenSuggestionIsRaisedOnceThatOneIsGone(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	first := e.note(e.Admin(), t, "note", "Excel at month-end.", dana, clocktest.Now(t).Add(-2*time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	e.mustSuggest(t, brain)
	second := e.note(e.Admin(), t, "note", "Excel again, still painful.", dana, clocktest.Now(t).Add(-time.Hour))
	if pass := e.mustSuggest(t, brain); pass.Raised != 0 {
		t.Fatalf("the second answer raised %d beside the open suggestion, want 0", pass.Raised)
	}
	calls := len(brain.prompts)

	e.WsExec(t, `INSERT INTO activity_retention_evidence
		       (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, first)
	e.WsExec(t, `UPDATE activity
		   SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		       restricted_until = now() + interval '1 year',
		       retention_class = 'commercial_correspondence', retention_class_at = now()
		 WHERE id = $1`, first)
	pass := e.mustSuggest(t, brain)
	if pass.Superseded != 1 || pass.Raised != 1 || len(brain.prompts) != calls {
		t.Fatalf("pass = %+v after %d new calls, want the old one retired and the stored answer raised without asking",
			pass, len(brain.prompts)-calls)
	}
	open, err := e.tags.OpenTagSuggestions(e.rep(e.Rep1, e.Team1), 10)
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %d (err %v), want 1", len(open), err)
	}
	cited, err := e.tags.GetTagSuggestion(e.rep(e.Rep1, e.Team1), open[0].ID)
	if err != nil || len(cited.Evidence) != 1 || cited.Evidence[0].ActivityID != second {
		t.Fatalf("the suggestion cites %+v (err %v), want the second note", cited.Evidence, err)
	}
}

func TestARefiledItemIsSuggestedOnItsNewRecordWithoutAskingAgain(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	bea := e.SeedContact(t, "Bea Buyer", &e.Rep1)
	note := e.note(e.Admin(), t, "note", "Excel at month-end.", dana, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}
	e.mustSuggest(t, brain)

	e.WsExec(t, `UPDATE activity_link SET contact_id = $2 WHERE activity_id = $1 AND contact_id = $3`, note, bea, dana)
	if pass := e.mustSuggest(t, brain); pass.Raised != 1 || len(brain.prompts) != 1 {
		t.Fatalf("after the refile the pass raised %d with %d calls, want 1 raised and no new call", pass.Raised, len(brain.prompts))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag_suggestion WHERE contact_id = $1 AND state = 'open'`, bea); n != 1 {
		t.Fatalf("%d open suggestions on the new record, want 1", n)
	}
}

func TestAnswersPastThePassBudgetAreRaisedOnLaterPasses(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	support := e.tag(t, "Premium support", "wants a named contact when something breaks", true)
	// Each item answers for both tags on its own record: two pairs an item.
	items := tagSuggestItemCap/2 + 10
	e.WsExec(t, `WITH pairs AS (SELECT uuidv7() AS contact, uuidv7() AS activity FROM generate_series(1, $1::int)),
		  c AS (INSERT INTO contact (id, full_name, source, captured_by) SELECT contact, 'Buyer', 'seed', 'system' FROM pairs),
		  a AS (INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		        SELECT activity, 'note', 'Call', 'Excel, and a named contact please.', $2, 'seed', 'system' FROM pairs)
		INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT activity, 'contact', contact FROM pairs`,
		items, clocktest.Now(t).Add(-time.Hour))
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration, "named contact": support}}

	if pass := e.mustSuggest(t, brain); pass.Raised != tagScoutSuggestionCap {
		t.Fatalf("the first pass raised %d, want the budget of %d", pass.Raised, tagScoutSuggestionCap)
	}
	calls := len(brain.prompts)
	if pass := e.mustSuggest(t, brain); pass.Raised != 2*items-tagScoutSuggestionCap || len(brain.prompts) != calls {
		t.Fatalf("the next pass raised %d with %d new calls, want the %d left over and no call",
			pass.Raised, len(brain.prompts)-calls, 2*items-tagScoutSuggestionCap)
	}
}

func TestAnItemNotEveryReaderMayReadIsNeverAskedBesideAnother(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	rep1 := e.rep(e.Rep1, e.Team1)
	at := clocktest.Now(t).Add(-time.Hour)
	private := e.note(rep1, t, "email", "Excel, privately.", dana, at)
	if _, err := e.Activities.SetAudience(rep1, ids.From[ids.ActivityKind](private),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail to its owner: %v", err)
	}
	e.note(e.Admin(), t, "note", "Thanks for lunch.", dana, at)
	e.note(e.Admin(), t, "note", "Agenda attached.", dana, at)
	brain := &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}

	e.mustSuggest(t, brain)
	for _, req := range brain.prompts {
		spans := fencedSpans(req.System, req.Messages[0].Content)
		for _, span := range spans {
			if span.id == private.String() && len(spans) != 1 {
				t.Fatalf("the owner-only mail was asked about beside %d other items", len(spans)-1)
			}
		}
	}
	if asked := len(brain.askedAbout()); asked != 3 {
		t.Fatalf("the model was asked about %d items, want all 3", asked)
	}
}

func TestADeclinedItemIsStoredWithNoTagAndItsSiblingRaises(t *testing.T) {
	e := setupTagScout(t)
	migration := e.tag(t, "Ledger migration", "move bookkeeping off spreadsheets", true)
	dana := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	bea := e.SeedContact(t, "Bea Buyer", &e.Rep1)
	at := clocktest.Now(t).Add(-time.Hour)
	hostile := e.note(e.Admin(), t, "note", "Excel "+hostileMarker, dana, at)
	e.note(e.Admin(), t, "note", "Excel at month-end.", bea, at)
	brain := &withholdingWhere{inner: &tagBrainStub{cues: map[string]ids.TagID{"Excel": migration}}}

	if pass := e.mustSuggest(t, brain); pass.Raised != 1 {
		t.Fatalf("the pass raised %d, want the sibling's suggestion alone", pass.Raised)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag_interest_read WHERE activity_id = $1 AND tag_ids = '{}'`, hostile); n != 1 {
		t.Fatalf("the declined item has %d tagless ledger rows, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM tag_suggestion WHERE contact_id = $1`, dana); n != 0 {
		t.Fatalf("the declined item raised %d suggestions, want 0", n)
	}
}
