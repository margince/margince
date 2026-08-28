// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the page-facts call ASKS, and the shape it must answer in.
//
// The two belong together and apart from the gate that judges the answer: the
// prompt describes each field in prose and the schema describes the same
// fields to the decoder, so a field named in one and missing from the other is
// a call that cannot answer what it was asked. Changing what a field means is
// therefore two edits in this file, never one — the entity block's `r` and `v`
// are the worked example, being two different authorities' numbers that a
// single loose description had let the model merge.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// pageFactsSystem is the per-page prompt. Small on purpose: the field
// menu and the guidance line are the whole instruction. The fence is the
// one wrapping the passages in the SAME call — its rule closes the prompt,
// because the boundary is minted per call and the model has to be told
// which marker it is looking at.
func pageFactsSystem(menu pageMenu, fence promptfence.Fence) string {
	var b strings.Builder
	b.WriteString("You extract company facts from ONE page of a company's website for a CRM. The page is given as numbered passages [s0], [s1], ….\n")
	b.WriteString(`Return ONLY a JSON object: {"facts":[...]`)
	if menu.people {
		b.WriteString(`,"people":[...]`)
	}
	if menu.entities {
		b.WriteString(`,"entities":[...]`)
	}
	b.WriteString("}.\n")
	if len(menu.factFields) > 0 {
		fmt.Fprintf(&b, "facts — one entry per distinct item: {\"f\":field,\"v\":value,\"e\":passage id}. Allowed fields: %s. %s\n",
			strings.Join(menu.factFields, ", "), menuGuidance(menu.factFields))
		b.WriteString("For list fields spell v as the item's name, then ' — ', then a short description when the page gives one. The item's NAME must appear in the passage you cite.\n")
	} else {
		b.WriteString("facts must be empty for this page.\n")
	}
	if menu.people {
		b.WriteString("people — ONLY people this page itself publishes: {\"n\":full name,\"r\":stated role,\"q\":the words tying them together,\"w\":the other people inside q,\"m\":email,\"l\":linkedin url,\"e\":passage id}. " +
			"r is the person's WHOLE title as printed — \"Senior Amazon Account-Manager\", never just \"Senior\". " +
			"q is a VERBATIM copy of the page, running from the role to the name or from the name to the role, unbroken — copy every word in between, change nothing, add nothing. " +
			"A page listing several people under one heading gives each of them a q that starts at that heading, and w then names the colleagues that q reaches over. " +
			"w is empty unless q prints somebody else; a name in q that w omits means the claim is refused. " +
			"When the page never states that THIS person holds THIS role, leave the person out entirely rather than guessing a q. " +
			"Include m or l ONLY when the page prints that exact address or URL — omit otherwise, NEVER guess.\n")
	}
	if menu.entities {
		b.WriteString("entities — EVERY distinct legal entity this legal page names: {\"n\":entity name,\"a\":registered address,\"r\":commercial-register entry,\"v\":VAT/tax number,\"e\":passage id}. " +
			"A legal notice states each entity as a block: give the address and the numbers printed WITH that entity's name, copied exactly as printed. " +
			"r and v are DIFFERENT identifiers issued by different authorities, so never put one in the other's place and never combine several into one string. " +
			"r is the court register entry — \"HRB 12345 B\", \"HRA 4711\", a companies-house number. " +
			"v is the tax identifier — a VAT ID like \"DE123456789\", a UID, a tax number. " +
			"a, r and v are ALWAYS present in your answer — use an empty string when the page states none for that entity, and never carry one entity's detail onto another. " +
			"A market, office or brand label (\"Acme Singapore\", \"DACH\") is NOT an entity: the entity is the registered company name printed under that label (\"Acme Pte. Ltd.\"). List every entity.\n")
	}
	b.WriteString("Cite the passage id that states each item. OMIT anything the page does not state — never guess.\n")
	b.WriteString(fence.Rule("page"))
	return b.String()
}

// menuGuidance narrows categoryGuidance to the fields the menu offers.
func menuGuidance(fields []string) string {
	present := map[string]bool{}
	for _, f := range fields {
		present[f] = true
	}
	var parts []string
	// EVERY category the vocabulary describes, "market" included. It was
	// missing, so a page offering company_size or served_industry reached the
	// model with no guidance for either — and company_size then collected
	// whatever looked numeric: a headcount that belonged in employee_range,
	// the company's own name, a revenue figure. The categories are listed in
	// the order the guidance reads best, not the order the map iterates.
	for _, category := range []string{companyWord, "offering", "market", "signal"} {
		for _, f := range people.OrganizationFactFields[category] {
			if present[f] {
				parts = append(parts, categoryGuidance[category])
				break
			}
		}
	}
	return strings.Join(parts, " ")
}

// pageFactsSchema pins the reply shape at generation: the field AND the
// snippet id are enums of exactly what this page offers.
func pageFactsSchema(menu pageMenu, snippetIDs []string) json.RawMessage {
	props := map[string]schema.Node{}
	required := []string{"facts"}
	factItem := map[string]schema.Node{
		"v": schema.String().Describe("The item's value."),
		"e": schema.Enum(snippetIDs...).Describe("The passage id that states it."),
	}
	if len(menu.factFields) > 0 {
		factItem["f"] = schema.Enum(menu.factFields...).Describe("Which fact field this is.")
		props["facts"] = schema.Array(schema.Object(factItem, "f", "v", "e"))
	} else {
		// The lane key stays present (one envelope shape for the shared
		// validator) but can only hold nothing.
		props["facts"] = schema.Array(schema.Object(factItem, "v", "e"))
	}
	if menu.people {
		props["people"] = schema.Array(schema.Object(map[string]schema.Node{
			"n": schema.String().Describe("The person's full name as printed."),
			"r": schema.String().Describe("The person's stated role."),
			"q": schema.String().Describe(
				"Copy the page's own words that give THIS person THIS role, " +
					"from the role to the name or the name to the role, exactly as printed " +
					"and with nothing left out in between. If the page never puts the two " +
					"together, omit the person."),
			"w": schema.String().Describe(
				"Every OTHER person printed inside q, separated by '; '. " +
					"Empty string when q names nobody else. Copy each name exactly as printed."),
			"m": schema.String().Describe("An email ONLY if this page prints it verbatim."),
			"l": schema.String().Describe("A LinkedIn URL ONLY if this page prints it verbatim."),
			"e": schema.Enum(snippetIDs...).Describe("The passage id naming the person."),
		}, "n", "r", "q", "w", "e"))
		required = append(required, "people")
	}
	if menu.entities {
		props["entities"] = schema.Array(schema.Object(map[string]schema.Node{
			"n": schema.String().Describe("The legal entity's name as printed."),
			"a": schema.String().Describe("Its registered address exactly as printed for THIS entity; empty string if the page states none."),
			"r": schema.String().Describe("Its commercial-register entry (HRB/HRA or the local equivalent) exactly as printed for THIS entity; empty string if the page states none."),
			"v": schema.String().Describe("Its VAT, UID or tax number exactly as printed for THIS entity; empty string if the page states none."),
			"e": schema.Enum(snippetIDs...).Describe("The passage id naming it."),
		}, "n", "a", "r", "v", "e"))
		required = append(required, "entities")
	}
	return schema.Must(schema.Object(props, required...))
}
