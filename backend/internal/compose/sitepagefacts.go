// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read's page-parallel fact lane: one SMALL call per
// fact-bearing page, each with a kind-routed field menu and the page's
// numbered passages, answered in compact records that cite a snippet id
// instead of quoting ({"f","v","e"} ≈ 30 output tokens per fact). Both
// the field name and the snippet id are SCHEMA ENUMS, so an unknown
// field or an uncitable id cannot even be generated; the gate resolves
// each citation and demands the value's name in the cited passage —
// the no-guess property, at a fraction of the tokens. The calls are
// independent, so the orchestrator fans them out concurrently on the
// fast routing tier: their latency IS the deep read's wall clock.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// pageMenu is what one page kind is asked for: the fact fields it may
// answer, and whether its call carries the people or legal-entity lanes.
type pageMenu struct {
	factFields []string
	people     bool
	entities   bool
}

// menuForKind routes a page kind to its menu; ok=false means the page
// makes NO call (boilerplate and unclassified pages state few facts and
// their calls would dominate cost, not quality).
func menuForKind(kind crmcontracts.SiteReadPageKind) (pageMenu, bool) {
	company := people.OrganizationFactFields[companyWord]
	offeringAndMarket := factFields("offering", "market")
	switch kind {
	case crmcontracts.SiteReadPageKindImpressum:
		// The imprint carries the people lane because German law puts the
		// board on it: §5 TMG requires a company to name its
		// Vertretungsberechtigte, so adesso.de/impressum prints five board
		// members and the supervisory board chair. That page is often the
		// ONLY place a large firm names anyone -- adesso publishes no team
		// directory the crawl reaches, and read without this lane it
		// yielded a hundred facts and zero people.
		//
		// The testimonial risk that shaped the About lane does not exist
		// here: an imprint quotes no customers.
		return pageMenu{factFields: company, entities: true, people: true}, true
	case crmcontracts.SiteReadPageKindContact:
		return pageMenu{factFields: company}, true
	case crmcontracts.SiteReadPageKindServices, crmcontracts.SiteReadPageKindProducts:
		return pageMenu{factFields: append(offeringAndMarket, people.FactTechnology)}, true
	case crmcontracts.SiteReadPageKindHome, crmcontracts.SiteReadPageKindAbout:
		// These pages keep the people lane: an about page's founders and
		// named staff are exactly the contacts worth having. What they must
		// not yield is the testimonial wall, and the published-email floor
		// is what separates the two — a company prints an address for the
		// person you should talk to, and never for the customer it is
		// quoting.
		//
		// They also carry the COMPANY category, and the omission was costly.
		// A home page's key-figures strip is where a company states its own
		// headcount and founding year — adesso.de prints "11500
		// Mitarbeitende" on its homepage, arvato.com "20,000 employees" on
		// its about page. Measured across 171 crawled companies, 51 of the 73
		// that print a headcount print it on one of these two kinds, and
		// employee_range was not on the menu for either: the field the
		// product promotes into the size_band column was unreachable exactly
		// where it is published.
		//
		// contact_email and phone ride along, which is right for these pages
		// too — a home page footer carries both as often as a contact page.
		// location comes with the category and is no longer appended by hand.
		return pageMenu{factFields: factFields(companyWord, "offering", "market", "signal"), people: true}, true
	case crmcontracts.SiteReadPageKindTeam:
		return pageMenu{people: true}, true
	default:
		return pageMenu{}, false
	}
}

func factFields(categories ...string) []string {
	var out []string
	for _, category := range categories {
		out = append(out, people.OrganizationFactFields[category]...)
	}
	return out
}

// factCategoryByField inverts the closed vocabulary: fact field names
// are globally unique (fitness-tested), so the model never states a
// category and the gate derives it.
var factCategoryByField = invertFactFields()

func invertFactFields() map[string]string {
	byField := map[string]string{}
	for category, fields := range people.OrganizationFactFields {
		for _, field := range fields {
			byField[field] = category
		}
	}
	return byField
}

// pageFactsReply is the compact JSON shape every page call answers in.
// pageFactsPerson is one claimed person in a page-facts reply: name, stated
// role, optional published email and LinkedIn, and the passage cited for it.
type pageFactsPerson struct {
	N string `json:"n"`
	R string `json:"r"`
	Q string `json:"q"`
	W string `json:"w"`
	M string `json:"m"`
	L string `json:"l"`
	E string `json:"e"`
}

type pageFactsReply struct {
	Facts []struct {
		F string `json:"f"`
		V string `json:"v"`
		E string `json:"e"`
	} `json:"facts"`
	People   []pageFactsPerson `json:"people"`
	Entities []struct {
		N string `json:"n"`
		A string `json:"a"`
		R string `json:"r"`
		V string `json:"v"`
		E string `json:"e"`
	} `json:"entities"`
}

// pageFactsShapeValid is the retry pipeline's parse check.
func pageFactsShapeValid(text string) error {
	var parsed pageFactsReply
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &parsed); err != nil {
		return fmt.Errorf("output must be {\"facts\":[...]} (+people/entities where asked): %w", err)
	}
	return nil
}

// pageFactsResult is one page's gate-surviving findings.
type pageFactsResult struct {
	url      string
	kind     crmcontracts.SiteReadPageKind
	facts    []people.DeepReadFact
	people   []sitePerson
	entities []corpusLegalEntity
}

// pageFactsExcerptRunes bounds what ONE page contributes to its own fact call.
//
// Derived from the profile lane's WHOLE-CALL budget, not from its per-page
// share, because that is the matching quantity: the profile lane spends
// profileExcerptBudgetRunes across a corpus in one call, and this lane issues
// one call per page, so a page here may cost what a corpus costs there.
//
// It has to be bounded by something. Without this the prompt was as long as
// whoever published the page chose to make it — up to webread's 1 MiB fetch
// cap, most of which survives StripTags on a text-heavy page. That spends a
// metered provider's tokens on a stranger's decision, and on a local provider
// it sizes the context window the adapter must allocate, which is how this was
// found.
const pageFactsExcerptRunes = profileExcerptBudgetRunes

// pageFactsExcerpt bounds one page for its own fact call, and answers how many
// runes it left behind.
//
// The count leaves with the page because the cap alone cannot say what was
// lost: a page one rune over the budget and one a hundred times over are read
// identically, and only the remainder tells them apart. Same reasoning the
// embed adapter's window report carries, for the same reason — a truncation
// nobody is told about is indistinguishable downstream from a complete read.
func pageFactsExcerpt(page crawlPage) (excerptPages, int) {
	runes := []rune(page.Text)
	if len(runes) <= pageFactsExcerptRunes {
		return excerptPages{page}, 0
	}
	unread := len(runes) - pageFactsExcerptRunes
	page.Text = string(runes[:pageFactsExcerptRunes])
	return excerptPages{page}, unread
}

// extractPageFacts runs one page's call on the fact lane and gates the
// reply. Pages whose kind has no menu return empty without a call.
func (x evidenceExtractor) extractPageFacts(ctx context.Context, page crawlPage) (pageFactsResult, error) {
	menu, ok := menuForKind(page.Kind)
	if !ok {
		return pageFactsResult{url: page.URL, kind: page.Kind}, nil
	}
	excerpt, unread := pageFactsExcerpt(page)
	if unread > 0 {
		// Both numbers, because the ratio is the finding: a page that
		// overruns by a paragraph and one whose facts are mostly past the cap
		// produce the same result and the same absent facts, and only the
		// remainder says which happened.
		slog.WarnContext(ctx, "a page is longer than this lane reads; its facts are extracted from the head of the page and anything stated below the cut is absent rather than refused",
			"url", page.URL, "kind", page.Kind, "read_runes", pageFactsExcerptRunes, "unread_runes", unread)
	}
	idx := newSnippetIndex(excerpt)
	if len(idx.refs) == 0 {
		return pageFactsResult{url: page.URL, kind: page.Kind}, nil
	}
	req := pageFactsRequest(menu, idx)
	brain := x.factCompleter()
	var resp model.Response
	var err error
	if structured, ok := brain.(validatedBrain); ok {
		resp, err = structured.CompleteValidated(ctx, req, pageFactsShapeValid)
	} else {
		resp, err = brain.Complete(ctx, req)
	}
	if err != nil {
		return pageFactsResult{}, err
	}
	result, dropped := gatePageFacts(resp.Text, page, menu, idx)
	x.reportDrops(ctx, page.URL, dropped)
	return result, nil
}

// pageFactsRequest builds the ONE call one page's fact lane sends. It is a
// pure function of the page's menu and its numbered passages, so the same
// request can be issued outside the deep read — by the certification lane —
// without re-creating it, because a re-creation certifies a copy rather than
// the prompt that ships.
//
// BOTH halves of the call are the menu's: the system prompt asks only for the
// lanes this page kind carries, and the schema's field enum offers only the
// fields it may answer with. Neither can be fixed — a pinned prompt would ask
// a catalog page for a legal entity, and a pinned schema would offer a legal
// notice fields its own menu never named.
//
// The id enum is this call's index for the same reason it is in the profile
// lane: the gate resolves every citation against the SAME numbering the model
// was shown, so one index feeds the prompt, the schema and the gate alike.
//
// The fence is minted here, per request: a boundary reused across calls is one
// some crawled site has already been shown, and every passage in this prompt is
// a site's own writing.
//
//promptlang:exempt the reply is field values printed on the page — emails, urls, counts — and gatePageFactList refuses one the page does not print verbatim, so a translated value is a dropped value.
//promptvoice:exempt the reply is field values printed on the page — emails, urls, counts — refused unless the page prints them verbatim.
func pageFactsRequest(menu pageMenu, idx snippetIndex) model.Request {
	fence := promptfence.New()
	return model.Request{
		System: pageFactsSystem(menu, fence),
		Messages: []model.Message{{
			Role: chatRoleUser,
			// renderNumbered names the page inside the fence; repeating the URL
			// here would put the site's own text in the prompt's voice.
			Content: idx.renderNumbered(fence),
		}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: pageFactsSchema(menu, idx.ids()),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// zeroedStat rejects a measurable claim whose measurement is zero. Sites
// animate their headline numbers up from 0, and a fetched page carries
// the pre-animation DOM — so "$10B+ GMV enabled", the figure a human
// sees, reaches extraction as "0 B + GMV enabled". It cites its passage
// honestly, which is why the citation gate passes it, and it is still a
// claim the company never made. Only quantified_outcome is affected:
// zero is meaningless for a stat and meaningful nowhere else.
func zeroedStat(field, value string) bool {
	if field != people.FactQuantifiedOutcome {
		return false
	}
	digits := strings.IndexFunc(value, unicode.IsDigit)
	if digits < 0 {
		return false // a claim with no number at all is not a zeroed one
	}
	for _, r := range value {
		if unicode.IsDigit(r) && r != '0' {
			return false
		}
	}
	return true
}

// gatePageFacts is the no-guess gate for one page's compact reply:
// closed vocabulary (schema-enforced, re-checked), resolvable citation,
// the value's NAME in the cited passage (±1 same-page join), people
// published-only, entities only from shallow legal pages. The stored
// evidence is the resolved passage — our own text, never the model's.
func gatePageFacts(modelText string, page crawlPage, menu pageMenu, idx snippetIndex) (pageFactsResult, []droppedFinding) {
	out := pageFactsResult{url: page.URL, kind: page.Kind}
	var parsed pageFactsReply
	if err := json.Unmarshal([]byte(ai.Unfence(modelText)), &parsed); err != nil {
		return out, []droppedFinding{{Lane: lanePageFacts, Reason: dropUnparseableReply}}
	}
	var dropped []droppedFinding
	drop := func(lane, field, value, reason string) {
		dropped = append(dropped, droppedFinding{Lane: lane, Field: field, Value: value, Reason: reason})
	}
	out.facts = gatePageFactList(parsed, page, menu, idx, drop)
	if menu.people {
		out.people = gatePagePeople(parsed, page, idx, drop)
	}
	if menu.entities {
		out.entities = gatePageEntities(parsed, page, idx, drop)
	}
	return out, dropped
}

func gatePageFactList(parsed pageFactsReply, page crawlPage, menu pageMenu, idx snippetIndex, drop func(lane, field, value, reason string)) []people.DeepReadFact {
	allowed := map[string]bool{}
	for _, f := range menu.factFields {
		allowed[f] = true
	}
	var out []people.DeepReadFact
	factIndex := map[string]int{}
	for _, f := range parsed.Facts {
		category := factCategoryByField[f.F]
		switch {
		case !allowed[f.F] || category == "":
			drop(lanePageFacts, f.F, f.V, dropUnknownField)
			continue
		case strings.TrimSpace(f.V) == "":
			drop(lanePageFacts, f.F, f.V, dropEmptyValue)
			continue
		}
		evidence, cited := idx.nameInCited(f.E, factName(f.V))
		if !cited {
			drop(lanePageFacts, f.F, f.V, dropValueNotInSnippet)
			continue
		}
		if zeroedStat(f.F, f.V) {
			drop(lanePageFacts, f.F, f.V, dropZeroedStat)
			continue
		}
		valueKey := ""
		if people.OrganizationFactMultiValue[f.F] {
			valueKey = people.NormalizeFactValueKey(f.V)
			if valueKey == "" {
				drop(lanePageFacts, f.F, f.V, dropEmptyValueKey)
				continue
			}
		}
		fact := people.DeepReadFact{
			Category: category, Field: f.F, Value: strings.TrimSpace(f.V), ValueKey: valueKey,
			EvidenceSnippet: evidence, SourceURL: page.URL, Confidence: gatedConfidence,
		}
		if _, dup := factIndex[factKey(fact)]; dup {
			drop(lanePageFacts, f.F, f.V, dropDuplicate)
			continue
		}
		factIndex[factKey(fact)] = len(out)
		out = append(out, fact)
	}
	return out
}

// factName is the dedupe/containment identity of a multi-value fact's
// value: the part before the " — " separator, the whole value otherwise.
func factName(value string) string {
	name, _, found := strings.Cut(value, factValueSeparator)
	if !found {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(name)
}

// factValueSeparator mirrors the people module's value spelling
// ("Name — short description").
const factValueSeparator = " — "

// gatedConfidence is the fixed confidence stamped on reference-evidence
// findings: the gate is binary (the citation resolves and carries the
// name, or the finding is dropped), so the model no longer self-grades —
// and the DB's (0,1] CHECK stays satisfied without a schema change.
const gatedConfidence = 1.0
