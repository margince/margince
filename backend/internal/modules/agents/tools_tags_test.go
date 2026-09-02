// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubTags answers the seam without a store, and records what it was asked.
type stubTags struct {
	applied, removed *taggingArgs
	ensured          *string
	vocabulary       []Tag
	listedArchived   *bool
	capped           bool
	taggable         []string
}

func (s stubTags) TaggableTypes() []string { return s.taggable }

// Both schemas advertise whatever the seam answers — the enum is the seam's
// vocabulary read at Spec time, not a list this package owns. The compose lane
// proves the composed wiring serves the store's list; this is the module-local
// half, that taggingSchema actually reads its argument.
func TestTheTaggingSchemasAdvertiseTheSeamsVocabulary(t *testing.T) {
	seam := stubTags{taggable: []string{"person", "project"}}
	for name, raw := range map[string]json.RawMessage{
		"apply_tag":  applyTag{tags: seam}.Spec().InputSchema,
		"remove_tag": removeTag{tags: seam}.Spec().InputSchema,
	} {
		if !bytes.Contains(raw, []byte(`"enum":["person","project"]`)) {
			t.Errorf("%s's record_type enum does not carry the seam's vocabulary: %s", name, raw)
		}
	}
}

func (s stubTags) ListTags(_ context.Context, includeArchived bool) ([]Tag, bool, error) {
	if s.listedArchived != nil {
		*s.listedArchived = includeArchived
	}
	return s.vocabulary, s.capped, nil
}

func (stubTags) EnsureTaggable(context.Context, string, ids.UUID) error { return nil }

func (s stubTags) FindTag(_ context.Context, name string) (ids.UUID, bool, error) {
	if s.ensured != nil {
		*s.ensured = name
	}
	return ids.NewV7(), true, nil
}

// ResolveTag stands in for a governed vocabulary: it answers for a name the
// workspace already holds and REFUSES anything else. Returning a fresh id for
// every name — which the stub it replaced did — would let a test claiming
// "apply_tag never creates" pass against a tool that creates.
// GetTag answers a fixed weight: these tests are about the tool's plumbing,
// and the counting itself is proved against a real database in the collections
// integration suite, where the numbers can be wrong.
func (s stubTags) GetTag(_ context.Context, tagID ids.UUID) (TagDetail, error) {
	return TagDetail{
		Tag:    Tag{TagID: tagID, Name: knownTagName},
		People: 2, Companies: 1, Deals: 0,
	}, nil
}

// RecordTags answers a fixed assignment: these tests are about the tool's
// plumbing, and the read itself is proved against a real database in the
// collections integration suite, where the permissions can be wrong.
func (s stubTags) RecordTags(_ context.Context, _ string, _ ids.UUID) (RecordTagsResult, error) {
	return RecordTagsResult{Tags: []RecordTagOnRecord{{
		Name: knownTagName, AssignedByKind: "human", AssignedAt: "2026-03-03T10:00:00Z",
	}}}, nil
}

func (s stubTags) RecordTagTypes() []string { return []string{"person", "organization", "deal"} }

func (s stubTags) ResolveTag(_ context.Context, name string) (ids.UUID, error) {
	if s.ensured != nil {
		*s.ensured = name
	}
	if name != knownTagName {
		return ids.UUID{}, errNoSuchTag
	}
	return ids.NewV7(), nil
}
func (s stubTags) ApplyTag(_ context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error {
	if s.applied != nil {
		*s.applied = taggingArgs{TagID: tagID, RecordType: entityType, RecordID: entityID}
	}
	return nil
}
func (s stubTags) RemoveTag(_ context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error {
	if s.removed != nil {
		*s.removed = taggingArgs{TagID: tagID, RecordType: entityType, RecordID: entityID}
	}
	return nil
}

// The tag IS the outcome of a capture flow ("add tag: K5 Conference 2026"),
// and no tool could perform it — so an assistant described work it had not
// done. Both halves are held here, because a vocabulary with no way back is
// the shape that made archive_record the only undo: retiring the word for
// everybody to correct one mistaken tagging.
func TestApplyAndRemoveReachTheSameTaggingBothWays(t *testing.T) {
	tag, org := ids.NewV7(), ids.NewV7()
	args := json.RawMessage(`{"tag_id":"` + tag.String() + `","record_type":"organization",` +
		`"record_id":"` + org.String() + `"}`)

	var applied, removed taggingArgs
	if _, err := (applyTag{tags: stubTags{applied: &applied}}).Handle(context.Background(), args); err != nil {
		t.Fatalf("applying answered %v", err)
	}
	if applied.TagID != tag || applied.RecordType != "organization" || applied.RecordID != org {
		t.Errorf("apply reached the seam as %+v, want the tag on that organization", applied)
	}
	if _, err := (removeTag{tags: stubTags{removed: &removed}}).Handle(context.Background(), args); err != nil {
		t.Fatalf("removing answered %v", err)
	}
	if removed != applied {
		t.Errorf("remove reached the seam as %+v, want the same tagging apply made", removed)
	}
}

// A NAME rather than an id is the capture flow's shape: "add tag: Champion" is
// one act to the person asking. Making them call a lookup verb first, only to
// hand its answer straight back, is a second call that exists for the
// surface's convenience rather than theirs.
func TestApplyTagTakesANameAndResolvesTheExistingWord(t *testing.T) {
	var resolved string
	var applied taggingArgs
	_, err := (applyTag{tags: stubTags{applied: &applied, ensured: &resolved}}).Handle(
		context.Background(),
		json.RawMessage(`{"tag_name":"`+knownTagName+`","record_type":"organization",`+
			`"record_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatalf("applying by name answered %v, want the tag resolved", err)
	}
	if resolved != knownTagName {
		t.Errorf("the seam was asked to resolve %q, want the name as given", resolved)
	}
	if applied.TagID.IsZero() {
		t.Error("the tagging carries no tag id, want the resolved one")
	}
}

// The governance rule, asserted where an agent would break it. The vocabulary
// is Admin and Ops's to extend; a tool that coined a word on a name it did not
// recognise would hand every agent — and through it every rep — the authority
// the governance exists to withhold. A misspelling would become a permanent
// second tag nobody chose.
func TestApplyTagRefusesAnUnknownNameRatherThanCoiningIt(t *testing.T) {
	var applied taggingArgs
	_, err := (applyTag{tags: stubTags{applied: &applied}}).Handle(
		context.Background(),
		json.RawMessage(`{"tag_name":"Champoin","record_type":"organization",`+
			`"record_id":"`+ids.NewV7().String()+`"}`))
	if err == nil {
		t.Fatal("a typo'd name was accepted; want a refusal, because accepting it creates a second tag nobody chose")
	}
	if !applied.TagID.IsZero() {
		t.Errorf("a tagging reached the seam as %+v after the name was refused; nothing may be written", applied)
	}
}

// Neither an id nor a name is nothing to tag with, and the refusal says which
// of the two to send.
func TestApplyTagRefusesWithNeitherIDNorName(t *testing.T) {
	_, err := (applyTag{tags: stubTags{}}).Handle(context.Background(),
		json.RawMessage(`{"record_type":"organization","record_id":"`+ids.NewV7().String()+`"}`))
	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("answered %v, want a BadArgsError naming tag_id or tag_name", err)
	}
}

// The target is authorized BEFORE a tag is created for it.
//
// Creating first left a live, audited word behind whenever the record turned
// out not to exist or to sit outside the caller's row scope — a write nobody
// asked for, produced by a call that then failed.
func TestApplyTagChecksTheRecordBeforeMintingAWord(t *testing.T) {
	var ensured string
	_, err := (applyTag{tags: refusingTaggable{ensured: &ensured}}).Handle(context.Background(),
		json.RawMessage(`{"tag_name":"K5 Conference 2026","record_type":"organization",`+
			`"record_id":"`+ids.NewV7().String()+`"}`))
	if err == nil {
		t.Fatal("applying to an unreachable record answered success, want the refusal")
	}
	if ensured != "" {
		t.Errorf("a tag named %q was created for a record the caller cannot tag", ensured)
	}
}

// A name that names nothing means the tagging is already absent — the state
// the caller asked for. It must not mint a word in order to remove it.
func TestRemoveTagByAnUnknownNameSucceedsWithoutCreatingIt(t *testing.T) {
	out, err := (removeTag{tags: noSuchTag{}}).Handle(context.Background(),
		json.RawMessage(`{"tag_name":"never used","record_type":"deal",`+
			`"record_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatalf("removing an unknown tag answered %v, want success", err)
	}
	var got TagAppliedResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Applied {
		t.Error("the result claims a tagging was applied")
	}
}

type refusingTaggable struct {
	stubTags
	ensured *string
}

func (r refusingTaggable) EnsureTaggable(context.Context, string, ids.UUID) error {
	return errNoSuchRecord
}

func (r refusingTaggable) GetTag(_ context.Context, tagID ids.UUID) (TagDetail, error) {
	return TagDetail{Tag: Tag{TagID: tagID, Name: knownTagName}}, nil
}

func (r refusingTaggable) RecordTags(_ context.Context, _ string, _ ids.UUID) (RecordTagsResult, error) {
	return RecordTagsResult{}, nil
}

func (r refusingTaggable) RecordTagTypes() []string {
	return []string{"person", "organization", "deal"}
}

func (r refusingTaggable) ResolveTag(_ context.Context, name string) (ids.UUID, error) {
	if r.ensured != nil {
		*r.ensured = name
	}
	if name != knownTagName {
		return ids.UUID{}, errNoSuchTag
	}
	return ids.NewV7(), nil
}

var errNoSuchRecord = errors.New("no such record in this workspace")

// knownTagName is the one word these stubs' vocabulary holds.
const knownTagName = "Champion"

var errNoSuchTag = errors.New("no tag by that name exists, and this tool does not create one")

type noSuchTag struct{ stubTags }

func (noSuchTag) FindTag(context.Context, string) (ids.UUID, bool, error) {
	return ids.UUID{}, false, nil
}

// The vocabulary had no door. apply_tag's own copy says to prefer a tag_id
// "you already hold", and nothing on the surface could produce one: create was
// human-only and the listing was declared by nobody. So the only reachable way
// to tag was to pass a NAME, which creates the word when the workspace has no
// such spelling — and a caller who cannot see the existing words guesses.
// "K5 Conference" beside "K5 Conference 2026" is not two tags, it is a
// vocabulary that has stopped being one.
func TestTheVocabularyCanBeReadBeforeAWordIsCoined(t *testing.T) {
	marketing := ids.NewV7()
	var askedForArchived bool
	tool := listTags{tags: stubTags{
		vocabulary:     []Tag{{TagID: marketing, Name: "K5 Conference 2026", Color: "#3366ff"}},
		listedArchived: &askedForArchived,
	}}

	raw, err := tool.Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("listing answered %v", err)
	}
	var got ListTagsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if len(got.Tags) != 1 {
		t.Fatalf("listed %+v, want the workspace's word", got.Tags)
	}
	// The id is the whole point: it is what apply_tag takes, and a name-only
	// answer would leave the caller guessing exactly as before.
	if got.Tags[0].TagID != marketing || got.Tags[0].Name != "K5 Conference 2026" {
		t.Errorf("the word reads %+v, want the id apply_tag takes beside its name", got.Tags[0])
	}
	if askedForArchived {
		t.Error("the default asked the store for archived words; retired words are opt-in")
	}
}

// Opt-in, and it reaches the store rather than being filtered here: the store
// is where `archived_at IS NULL` lives, and a second spelling of that rule in
// this module is a second place for it to drift.
func TestAskingForRetiredWordsReachesTheStore(t *testing.T) {
	var askedForArchived bool
	tool := listTags{tags: stubTags{listedArchived: &askedForArchived}}
	if _, err := tool.Handle(context.Background(),
		json.RawMessage(`{"include_archived":true}`)); err != nil {
		t.Fatalf("listing answered %v", err)
	}
	if !askedForArchived {
		t.Error("include_archived did not reach the store, so the argument is decoration")
	}
}

// An empty workspace is an ANSWER, not an error, and it must decode as `[]`
// rather than `null`: a caller handed null reads it as a failed read and says
// the vocabulary could not be listed, when the truth is that nobody has coined
// a word yet.
func TestAnEmptyVocabularyAnswersAsAList(t *testing.T) {
	raw, err := (listTags{tags: stubTags{}}).Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("listing answered %v", err)
	}
	if !bytes.Contains(raw, []byte(`"tags":[]`)) {
		t.Errorf("an empty workspace answered %s, want an empty list", raw)
	}
}

// A retired word rides back MARKED. apply_tag will not reuse one — EnsureTag
// treats a name whose only holder is archived as a conflict, deliberately, so
// that retiring a word is not undone by a coincidence of spelling — and a
// caller shown the word without the flag would read that refusal as a bug.
func TestARetiredWordSaysSo(t *testing.T) {
	retired := ids.NewV7()
	tool := listTags{tags: stubTags{
		vocabulary: []Tag{{TagID: retired, Name: "Q1 Push", Archived: true}},
	}}
	raw, err := tool.Handle(context.Background(), json.RawMessage(`{"include_archived":true}`))
	if err != nil {
		t.Fatalf("listing answered %v", err)
	}
	var got ListTagsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if len(got.Tags) != 1 || !got.Tags[0].Archived {
		t.Errorf("listed %+v, want the retired word marked archived", got.Tags)
	}
}

// A capped read SAYS it was capped, and this is the finding that makes the
// tool honest rather than merely present.
//
// The store caps the vocabulary read, and the caller's whole reason for asking
// is to learn whether a word already exists. A capped list handed over as the
// vocabulary answers "no such tag" for everything past the cap — so the caller
// coins the duplicate that reading the vocabulary was meant to prevent, and
// nothing anywhere says why.
func TestACappedVocabularySaysItIsCapped(t *testing.T) {
	tool := listTags{tags: stubTags{
		vocabulary: []Tag{{TagID: ids.NewV7(), Name: "Key Account"}},
		capped:     true,
	}}
	raw, err := tool.Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("listing answered %v", err)
	}
	var got ListTagsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if !got.Truncated {
		t.Error("a capped read answered as the whole vocabulary; a caller past the cap " +
			"is told a word does not exist when it does")
	}
}

// The flag is absent when nothing was cut, so `truncated` means something when
// it appears rather than riding every answer as noise.
func TestAWholeVocabularyDoesNotClaimToBeCut(t *testing.T) {
	raw, err := (listTags{tags: stubTags{
		vocabulary: []Tag{{TagID: ids.NewV7(), Name: "Inbound"}},
	}}).Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("listing answered %v", err)
	}
	if bytes.Contains(raw, []byte(`"truncated"`)) {
		t.Errorf("a complete vocabulary answered %s, want no truncation flag", raw)
	}
}
