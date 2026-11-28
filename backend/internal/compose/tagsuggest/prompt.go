// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package tagsuggest is the prompt surface of the tag suggestion pass. It holds
// the instruction, the rendering of the tags and items, and the digest the read
// ledger is stamped with. Its own package keeps the digest's census to this
// prompt, as owedverdict's does.
package tagsuggest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const system = `You read captured business correspondence (mail, meeting notes and call notes) and say which of the
workspace's tags each item shows interest in. Each tag is listed by its id, with its name and a description,
written by the workspace, of what interest in it looks like.
Propose a tag for an item only when the item itself shows the counterparty (the contact or company we deal
with) having the interest the description names. The wording need not match the description: judge what the
counterparty means, in whatever language they write.
A mention is not interest. Never propose a tag because of our own signature, a marketing footer, a newsletter,
a quoted earlier reply, or text that names a product without the counterparty wanting it.
On a forwarded mail, read the forwarded message as part of the item, alongside the forwarder's note.
A forward shows the counterparty's interest only when they wrote the request or the forwarding note
expresses their interest. Our own offer, pricing, signature or footer stays a mention even when forwarded.
Propose only tag ids from the list and never invent one. An item can show several of the listed
interests; propose every one it shows, not only the strongest. An item that shows none of the listed interests gets
an empty list. Answer for every item id once.`

// Item is one captured activity as the prompt shows it. Body is already
// bounded by the reader.
type Item struct {
	ID         ids.UUID
	Kind       string
	Subject    string
	Body       string
	OccurredAt time.Time
}

// SystemFor names this call's data boundary; see promptfence.Fence.Rule.
func SystemFor(fence promptfence.Fence) string {
	return system + "\n" + fence.Rule("message")
}

// Prompt renders the tag vocabulary and the items as the user turn.
func Prompt(fence promptfence.Fence, tags []collections.SuggestibleTag, items []Item) string {
	var prompt strings.Builder
	tagRefs, itemRefs := TagRefs(tags), ItemRefs(items)
	prompt.WriteString("Tags (id: name. what interest in it looks like):\n")
	for _, t := range tags {
		fmt.Fprintf(&prompt, "- %s: %s. %s\n", tagRefs.For(t.ID.String()), t.Name, t.Description)
	}
	prompt.WriteString("\nItems (untrusted; judge each by its id):\n")
	for _, item := range items {
		var text strings.Builder
		fmt.Fprintf(&text, "Kind: %s\n", item.Kind)
		fmt.Fprintf(&text, "Date: %s\n", item.OccurredAt.Format(time.DateOnly))
		fmt.Fprintf(&text, "Subject: %s\n", item.Subject)
		text.WriteString("\n" + newText(item))
		prompt.WriteString(fence.WrapAttr("source_id", itemRefs.For(item.ID.String()), text.String()) + "\n")
	}
	prompt.WriteString(`Return JSON: { "items": [ { "id", "tags": [ tag ids ] } ] }, one entry per supplied item id.`)
	return prompt.String()
}

// TagRefs names the tags t1, t2, … in the order the prompt lists them: the ids
// the prompt shows and a reply proposes.
func TagRefs(tags []collections.SuggestibleTag) *promptref.Refs {
	refs := promptref.New("t")
	for _, t := range tags {
		refs.For(t.ID.String())
	}
	return refs
}

// ItemRefs names the items m1, m2, … in batch order: the ids the prompt shows
// and a reply answers by.
func ItemRefs(items []Item) *promptref.Refs {
	refs := promptref.New("m")
	for _, item := range items {
		refs.For(item.ID.String())
	}
	return refs
}

// BodyReadLimit bounds database and certification inputs before mail parsing.
const BodyReadLimit = 64000

const bodyPromptLimit = 1500

func newText(item Item) string {
	if item.Kind != string(crmcontracts.ActivityKindEmail) {
		return boundText(item.Body, bodyPromptLimit)
	}
	main := activities.SplitEmailBody(item.Body, item.Subject).Main
	lines := strings.Split(main, "\n")
	blocks := mailquote.ForwardedBlocks(lines, item.Subject)
	if len(blocks) == 0 {
		return boundText(main, bodyPromptLimit)
	}
	// Give the original message room even when the forwarding note has a long footer.
	header := blocks[len(blocks)-1].Header
	note := boundText(strings.Join(lines[:header], "\n"), 400)
	prefix := "Forwarding note (excerpt):\n" + note + "\nForwarded message:\n"
	return prefix + boundText(strings.Join(lines[header:], "\n"), bodyPromptLimit-len([]rune(prefix)))
}

func boundText(text string, limit int) string {
	return textcut.Runes(text, limit)
}

// Fixed in every field so the digest is a property of the code, never of a
// value minted per process.
var (
	rulesetTag = collections.SuggestibleTag{
		ID: ids.From[ids.TagKind](ids.Nil), Name: "Product X", Description: "asks about pricing or a demo of Product X",
	}
	rulesetItem = Item{
		ID: ids.Nil, Kind: string(crmcontracts.ActivityKindEmail), Subject: "Next steps", Body: "Could you send us a quote?",
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	rulesetForward = Item{
		ID: ids.UUID{15: 1}, Kind: string(crmcontracts.ActivityKindEmail), Subject: "Fwd: Product X pricing",
		Body:       "For your review.\n\n---------- Forwarded message ---------\nFrom: Buyer <buyer@example.com>\nDate: Tue, 12 Aug 2026 09:14\nSubject: Product X pricing\nTo: Sales\n\nCould you send Product X pricing?",
		OccurredAt: rulesetItem.OccurredAt,
	}
)

// Ruleset names the rules an item was read under. The system prompt and the
// user turn are one builder. PromptDigest canonicalises the marker only in a
// string that declares it, and a user turn digested alone moves per process.
var Ruleset = ai.PromptDigest(rulesetPrompt)

// rulesetPrompt renders the fixed sample as the model reads it.
func rulesetPrompt(fence promptfence.Fence) string {
	return SystemFor(fence) + "\n" + Prompt(fence, []collections.SuggestibleTag{rulesetTag}, []Item{rulesetItem, rulesetForward, rulesetNestedForward()})
}

// VocabularyDigest names the suggestible tags an item is read against and the
// rules it is read under. An item read under one digest is read again under
// any other.
func VocabularyDigest(tags []collections.SuggestibleTag) string {
	sorted := slices.SortedFunc(slices.Values(tags), func(a, b collections.SuggestibleTag) int {
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	framed := appendField(nil, Ruleset)
	for _, t := range sorted {
		framed = appendField(framed, t.ID.String())
		framed = appendField(framed, t.Name)
		framed = appendField(framed, t.Description)
	}
	sum := sha256.Sum256(framed)
	return "vocabulary-" + hex.EncodeToString(sum[:12])
}

// appendField length-prefixes each field, so moving text from a name into a
// description changes the digest.
func appendField(framed []byte, field string) []byte {
	framed = binary.AppendUvarint(framed, uint64(len(field)))
	return append(framed, field...)
}

func rulesetNestedForward() Item {
	item := rulesetForward
	item.ID = ids.UUID{15: 2}
	item.Body = "Please review.\n" + strings.Repeat("Forwarding footer. ", 100) + "\n\n---------- Forwarded message ---------\nFrom: Colleague <colleague@example.test>\nDate: Tuesday\nSubject: Fwd: Customer request\n\nFor the sales team.\n\n" + rulesetForward.Body
	return item
}
