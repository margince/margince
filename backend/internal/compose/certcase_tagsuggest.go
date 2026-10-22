// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for tag_suggest/interest. The request comes from
// tagSuggestRequest and the reply is judged by validateTagSuggestPayload, the
// builder and validator the pass uses. The expectation names each item's tags
// by position and by the fixture's own tag keys, because Prepare mints every id.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/tagsuggest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// tagSuggestCertDate dates every fixture item, so the request a run builds is
// a function of the corpus alone.
var tagSuggestCertDate = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

// tagSuggestFixture is one batch and the vocabulary it is read against.
type tagSuggestFixture struct {
	Tags  []tagSuggestFixtureTag  `json:"tags"`
	Items []tagSuggestFixtureItem `json:"items"`
}

// tagSuggestFixtureTag is one suggestible tag. Key names it in the expectation
// and never reaches the prompt.
type tagSuggestFixtureTag struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type tagSuggestFixtureItem struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// tagSuggestCases serves the one site that reads items against the tags.
type tagSuggestCases struct{}

func (tagSuggestCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskTagSuggest, Variant: "interest", Kind: ai.SiteKindOneShot}
}

// CertifiedScope is the one call: a declined batch is asked again item by item,
// and a run here measures the batch answer only.
func (tagSuggestCases) CertifiedScope() string { return aitasks.ScopeSingleCall }

// Prepare mints a tag id per fixture tag and an activity id per item. An
// answer can then carry them only by reading this call's prompt.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (tagSuggestCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var batch tagSuggestFixture
	if err := json.Unmarshal(fixture, &batch); err != nil {
		return nil, fmt.Errorf("tag_suggest/interest: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableTagSuggestBatch(batch); err != nil {
		return nil, err
	}
	var want [][]string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("tag_suggest/interest: the expected answer is not a list of tag keys per item: %w", err)
	}
	c := &tagSuggestCase{keyOf: map[string]string{}, expected: want}
	for _, t := range batch.Tags {
		tag := collections.SuggestibleTag{ID: ids.New[ids.TagKind](), Name: t.Name, Description: t.Description}
		c.tags = append(c.tags, tag)
		c.keyOf[tag.ID.String()] = t.Key
	}
	for _, item := range batch.Items {
		c.items = append(c.items, tagsuggest.Item{
			ID: ids.NewV7(), Kind: item.Kind, Subject: item.Subject, Body: item.Body, OccurredAt: tagSuggestCertDate,
		})
	}
	if err := c.refuseUnreachableExpectation(); err != nil {
		return nil, err
	}
	return c, nil
}

// refuseUnreadableTagSuggestBatch names a batch the pass could never send. It
// lacks a tag or an item, holds more items than one call reads, has a body past
// the read's bound, or reuses a key.
func refuseUnreadableTagSuggestBatch(batch tagSuggestFixture) error {
	if len(batch.Tags) == 0 || len(batch.Items) == 0 {
		return errors.New("tag_suggest/interest: the fixture needs at least one tag and one item")
	}
	if len(batch.Items) > tagSuggestBatchSize {
		return fmt.Errorf("tag_suggest/interest: the fixture carries %d items, but one call reads at most %d",
			len(batch.Items), tagSuggestBatchSize)
	}
	for i, item := range batch.Items {
		if n := utf8.RuneCountInString(item.Body); n > tagSuggestBodyLimit {
			return fmt.Errorf("tag_suggest/interest: item %d carries %d characters, but the read truncates every body to %d",
				i+1, n, tagSuggestBodyLimit)
		}
	}
	keys := map[string]bool{}
	for _, t := range batch.Tags {
		if t.Key == "" || keys[t.Key] {
			return fmt.Errorf("tag_suggest/interest: tag key %q is empty or repeated", t.Key)
		}
		keys[t.Key] = true
	}
	return nil
}

// tagSuggestCase is one batch ready to be read, closed over the minted ids.
type tagSuggestCase struct {
	tags     []collections.SuggestibleTag
	items    []tagsuggest.Item
	keyOf    map[string]string // minted tag id -> fixture key
	expected [][]string
}

// refuseUnreachableExpectation names an expectation no valid reply can meet.
func (c *tagSuggestCase) refuseUnreachableExpectation() error {
	if len(c.expected) != len(c.items) {
		return fmt.Errorf("tag_suggest/interest: the scenario expects %d answers for %d items",
			len(c.expected), len(c.items))
	}
	known := map[string]bool{}
	for _, key := range c.keyOf {
		known[key] = true
	}
	for i, keys := range c.expected {
		for _, key := range keys {
			if !known[key] {
				return fmt.Errorf("tag_suggest/interest: item %d expects tag %q, which the fixture does not list", i+1, key)
			}
		}
	}
	return nil
}

// Run issues the one request this site sends, bare. The pass's shape retry and
// split would certify an answer given after being told to try again.
func (c *tagSuggestCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := tagSuggestRequest(c.tags, c.items)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("tag_suggest/interest: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate parses and validates the reply as the pass does, and only then asks
// whether each item carries the tags the scenario expects.
func (c *tagSuggestCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	answers, err := parseTagSuggestReply(trace.Output, c.tags, c.items)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if disagreements := c.disagreements(answers); len(disagreements) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(disagreements, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// disagreements names every item whose tags differ from the expectation, by
// position and fixture key.
func (c *tagSuggestCase) disagreements(answers []tagSuggestAnswer) []string {
	answered := map[string][]string{}
	for _, a := range answers {
		answered[a.ID] = a.Tags
	}
	var out []string
	for i, item := range c.items {
		got := c.keys(answered[item.ID.String()])
		want := slices.Sorted(slices.Values(c.expected[i]))
		if !slices.Equal(got, want) {
			out = append(out, fmt.Sprintf("item %d is tagged %q where the scenario expects %q", i+1, got, want))
		}
	}
	return out
}

// keys maps answered tag ids to fixture keys, sorted and without repeats.
func (c *tagSuggestCase) keys(tagIDs []string) []string {
	out := make([]string, 0, len(tagIDs))
	for _, id := range tagIDs {
		out = append(out, c.keyOf[id])
	}
	slices.Sort(out)
	return slices.Compact(out)
}
