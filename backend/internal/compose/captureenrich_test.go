// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The §2.9 source-window rule as a table: the signature block is the
// TRAILING non-quoted lines — quoted history is never identity evidence,
// and padding never counts as content.

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// Everything this prompt carries is the sender's own writing — their name, their
// address, the trailing lines of their mail — and the only thing that keeps it
// out of the instruction region is a marker minted for THIS call and named in
// THIS call's system prompt. A request that fences under some other marker, or
// repeats the text beside the span, hands the instructions to whoever wrote the
// mail.
func TestSignatureEnrichRequestFencesEveryUntrustedFieldUnderTheMarkerItDeclares(t *testing.T) {
	cand := contacts.SignatureCandidate{
		FullName:   "Bob Contact",
		Email:      "bob@acme.example",
		ActivityID: ids.NewV7(),
		Body:       "Thanks!\nBest,\nBob Contact\nCTO, Acme GmbH\n+49 30 1234567",
	}
	lines := signatureBlock(cand.Body)

	req := signatureEnrichRequest(cand, lines)

	marker, declared := promptfence.MarkerIn(req.System)
	if !declared {
		t.Fatalf("the signature-enrich system prompt declares no data boundary: %q", req.System)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("got %d messages, want the single user turn", len(req.Messages))
	}
	content := req.Messages[0].Content
	openTag, closeTag := "<"+marker+">", "</"+marker+">"
	signatureTag := "<" + marker + ` source_id="` + cand.ActivityID.String() + `">`
	if !strings.Contains(content, signatureTag+lines+closeTag) {
		t.Errorf("the signature window is not opened under the declared marker keyed by its activity:\n%s", content)
	}
	if !strings.Contains(content, openTag+"Name: "+cand.FullName+"\nEmail: "+cand.Email+closeTag) {
		t.Errorf("the contact's own name and address are not inside the boundary:\n%s", content)
	}
	// Containment is not a question of membership: a prompt that keeps the fence
	// and ALSO repeats the text beside it puts that copy in the instruction region
	// while "is it inside?" stays true. So the assertion is on what the prompt
	// says in its OWN voice.
	instructions := outsideEverySpan(content, marker)
	for _, text := range []string{cand.FullName, cand.Email, "CTO, Acme GmbH", "+49 30 1234567"} {
		if strings.Contains(instructions, text) {
			t.Errorf("untrusted text %q reaches the instruction region:\n%s", text, instructions)
		}
	}
}

// outsideEverySpan is the prompt with every fenced span removed — what is left
// is what the model reads as instruction. An unclosed span keeps its text, which
// is the leak it would be.
func outsideEverySpan(content, marker string) string {
	var b strings.Builder
	closeTag := "</" + marker + ">"
	for rest := content; ; {
		openAt := strings.Index(rest, "<"+marker)
		if openAt < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:openAt])
		closeAt := strings.Index(rest[openAt:], closeTag)
		if closeAt < 0 {
			b.WriteString(rest[openAt:])
			return b.String()
		}
		rest = rest[openAt+closeAt+len(closeTag):]
	}
}

// A fence's scope is one call. A marker a previous sender was shown is a marker
// they can spell, so reusing one would give away the only thing they cannot
// forge.
func TestSignatureEnrichRequestMintsAFreshBoundaryPerCall(t *testing.T) {
	cand := contacts.SignatureCandidate{FullName: "Bob Contact", ActivityID: ids.NewV7()}

	first, declared := promptfence.MarkerIn(signatureEnrichRequest(cand, "CTO, Acme GmbH").System)
	if !declared {
		t.Fatal("the signature-enrich system prompt declares no data boundary")
	}
	second, declared := promptfence.MarkerIn(signatureEnrichRequest(cand, "CTO, Acme GmbH").System)
	if !declared {
		t.Fatal("the second signature-enrich system prompt declares no data boundary")
	}
	if first == second {
		t.Errorf("two signature-enrich requests share the boundary %q", first)
	}
}

func TestSignatureBlockWindow(t *testing.T) {
	t.Run("quoted history is excluded", func(t *testing.T) {
		body := "Thanks!\n> On Tue, Alice wrote:\n> old text\nBest,\nBob Contact\nCTO, Acme GmbH\n+49 30 1234567"
		got := signatureBlock(body)
		if strings.Contains(got, "old text") {
			t.Fatalf("quoted history leaked into the window: %q", got)
		}
		if !strings.Contains(got, "CTO, Acme GmbH") {
			t.Fatalf("signature line missing from the window: %q", got)
		}
	})

	t.Run("only the trailing lines survive a long body", func(t *testing.T) {
		var b strings.Builder
		for i := range 40 {
			fmt.Fprintf(&b, "prose line %d\n", i)
		}
		b.WriteString("Jane Doe\nHead of Ops\n")
		got := signatureBlock(b.String())
		if lines := strings.Count(got, "\n") + 1; lines > signatureLineCount {
			t.Fatalf("window holds %d lines, cap is %d", lines, signatureLineCount)
		}
		if !strings.HasSuffix(got, "Head of Ops") {
			t.Fatalf("window must end at the body's tail: %q", got)
		}
	})

	t.Run("an all-quoted body yields nothing", func(t *testing.T) {
		if got := signatureBlock("> a\n> b\n"); got != "" {
			t.Fatalf("all-quoted body produced %q, want empty", got)
		}
	})
}

// The field name the shape validator rejects is MODEL output, so a sender who
// steered the model chose it. It is logged and, on a §5.2 retry, appended back
// into the prompt — bounded at both exits or it is a writing surface.
func TestSignatureShapeValidationDoesNotEchoUnboundedModelText(t *testing.T) {
	flood := strings.Repeat("A", 100_000)
	payload := fmt.Sprintf(`{"fields":[{"field":%q,"value":"x","evidence_snippet":"y"}]}`, flood)

	err := signatureShapeValid(payload)
	if err == nil {
		t.Fatal("a field outside the allowed set was accepted")
	}
	if len(err.Error()) > 500 {
		t.Fatalf("the validation error is %d bytes — model-chosen text must be clamped before it reaches a log or the next prompt", len(err.Error()))
	}
}

func TestASignatureNamingSomebodyElseIsNotReadAsTheirs(t *testing.T) {
	t.Parallel()
	ann := contacts.SignatureCandidate{FullName: "Ann Smith", Email: "ann@example.test"}
	for name, block := range map[string]string{
		// The substring trap: "joanne" contains "ann", so a match that is not
		// word-bounded reads Joanne's title onto Ann.
		"a longer name containing theirs": "Viele Grüße\nJoanne Brown\nCEO",
		"a different contact entirely":    "Best\nMarcus Greven\nPartner Manager",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if signatureNamesContact(block, ann) {
				t.Errorf("a block naming somebody else was read as %q's own: %q", ann.FullName, block)
			}
		})
	}
}

func TestAShortNameStillNamesItsOwnSignature(t *testing.T) {
	t.Parallel()
	// Every token under three characters. A minimum length would lock this
	// contact out of enrichment for ever, whatever they sign.
	li := contacts.SignatureCandidate{FullName: "Li Bo"}
	if !signatureNamesContact("Regards\nLi Bo\nCEO", li) {
		t.Error("a contact whose name is short cannot prove their own signature is theirs")
	}
	// And the whole-word rule still holds for them.
	if signatureNamesContact("Regards\nLiam Bosch\nCEO", li) {
		t.Error("a longer name containing theirs was read as their own signature")
	}
}

func TestTheirOwnSignatureIsRead(t *testing.T) {
	t.Parallel()
	// The positive control: without it, a build refusing everybody would pass
	// both tests above.
	cand := contacts.SignatureCandidate{FullName: "Judith Andresen", Email: "judith@example.test"}
	if !signatureNamesContact("Viele Grüße\nJudith Andresen\nGeschäftsführerin", cand) {
		t.Error("a contact's own signature was refused")
	}
	// By address alone, for a signature that prints the mail and not the name.
	if !signatureNamesContact("Viele Grüße\njudith@example.test", cand) {
		t.Error("a signature carrying only their address was refused")
	}
}

// The vocabulary is spelled four times — the gate map, the system prompt's
// allowed list, the user prompt's field list and the reply schema's enum — and
// the three the MODEL reads decide what it emits. A key present in one and
// absent from another is not a compile error: the model picks the word it was
// offered and the gate drops it, so a signature that stated the field reads as
// a signature that stated nothing.
//
// The list is derived from enrichFieldNames rather than written out again here,
// because a second copy in the test is the same defect the test exists to catch.
func TestSignatureEnrichSpellsOneVocabularyEverywhere(t *testing.T) {
	want := make([]string, 0, len(enrichFieldNames))
	for name := range enrichFieldNames {
		want = append(want, name)
	}
	sort.Strings(want)

	cand := contacts.SignatureCandidate{
		FullName:   "Bob Contact",
		Email:      "bob@acme.example",
		ActivityID: ids.NewV7(),
		Body:       "Best,\nBob Contact\nCTO, Acme GmbH",
	}
	req := signatureEnrichRequest(cand, signatureBlock(cand.Body))

	for _, spelling := range []struct {
		where string
		names []string
	}{
		{"the system prompt's allowed list", commaListAfter(req.System, "Allowed fields ONLY: ")},
		{"the user prompt's field list", quotedListAfter(req.Messages[0].Content, "Fields to extract when stated: ")},
		{"the reply schema's enum", schemaFieldEnum(t, signatureEnrichSchema())},
	} {
		got := append([]string(nil), spelling.names...)
		sort.Strings(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s names %v, but the gate admits %v — the model is offered a word the gate drops, or denied one it accepts",
				spelling.where, got, want)
		}
	}
}

// A job title is one thing, and `title` is the word for it: it is the only key
// contacts.observedFieldColumn mirrors onto contact.title. `role` named the same
// thing and mirrored nothing, so whichever the model happened to pick decided
// whether the screen showed the title or a dash.
func TestSignatureEnrichOffersNoSecondWordForAJobTitle(t *testing.T) {
	for _, synonym := range []string{"role", "position", "job_title"} {
		if enrichFieldNames[synonym] {
			t.Errorf("the vocabulary admits both %q and %q for a job title; only %q reaches contact.title",
				fieldTitle, synonym, fieldTitle)
		}
	}
	if !enrichFieldNames[fieldTitle] {
		t.Fatalf("the vocabulary no longer admits %q, which is the key a job title is recorded under", fieldTitle)
	}
}

// commaListAfter reads the comma-separated run that follows marker and ends at
// the first period — the shape the system prompt states its vocabulary in.
func commaListAfter(text, marker string) []string {
	_, rest, found := strings.Cut(text, marker)
	if !found {
		return nil
	}
	list, _, found := strings.Cut(rest, ".")
	if !found {
		return nil
	}
	names := strings.Split(list, ",")
	for i, name := range names {
		names[i] = strings.TrimSpace(name)
	}
	return names
}

// quotedListAfter reads the JSON array of strings that follows marker.
func quotedListAfter(text, marker string) []string {
	_, rest, found := strings.Cut(text, marker)
	if !found {
		return nil
	}
	line, _, _ := strings.Cut(rest, "\n")
	var names []string
	if err := json.Unmarshal([]byte(line), &names); err != nil {
		return nil
	}
	return names
}

// schemaFieldEnum reads the field key's permitted values out of the generated
// reply schema, so the assertion reads what the provider is actually sent.
func schemaFieldEnum(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var doc struct {
		Properties struct {
			Fields struct {
				Items struct {
					Properties struct {
						Field struct {
							Enum []string `json:"enum"`
						} `json:"field"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"fields"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("reading the signature-enrich reply schema: %v", err)
	}
	return doc.Properties.Fields.Items.Properties.Field.Enum
}
