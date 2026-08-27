// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Navigation is not evidence.
//
// The per-page fact lane reads a page as it arrives, which is before the crawl
// exists — so unlike the profile lane it cannot have its chrome removed before
// the prompt is built. What it can do is answer for what it found once the
// corpus is whole, which is where this runs.
//
// Two things a live read produced made the case. A legal-entity candidate came
// back cited to the site's flattened mega-menu:
//
//	Imprint | Gradion | Gradion Solutions Products Industries About English
//	Contact Us Solutions Products Industries About English Deutsch Tiếng Việt …
//
// True, and useless: the reader is asked to choose between entities, and that
// passage names all of them. And the language switcher's own labels came back
// as `language` facts scored 100 — true statements about the WEBSITE, not
// about the business, pre-selected into the company context with everything
// else.
//
// One rule covers both, because both are the same mistake: a finding whose
// evidence lies inside a block the crawl carries on most of its pages is
// supported by the site's furniture rather than by anything the company said.
//
// Dropped rather than stripped of its evidence. A candidate with no support is
// worse on that screen than one fewer candidate: the decision is which entity
// this installation is for, and an option a reader cannot judge is an option
// that makes the judgement harder. Every drop is reported through the same
// trail a gate refusal takes, so nothing disappears without saying so.

import (
	"strings"
)

const (
	// chromeEvidenceLane names these drops in the trail the read reports.
	chromeEvidenceLane = "chrome-evidence"
	// The two finding kinds that are not a field of the company profile, named
	// so the trail says what was dropped rather than leaving it to the value.
	chromeEvidencePersonField = "person"
	chromeEvidenceEntityField = "legal_entity"
)

// suppressChromeEvidence removes the findings whose evidence is the site's own
// navigation, and answers what it dropped.
//
// The chrome blocks come from the SAME measurement the profile lane's excerpt
// uses, rather than from a second reading of what furniture is. A crawl too
// small or too large to measure yields no blocks and nothing is dropped —
// failing open, because a missing finding is harder to notice than a noisy
// one.
func suppressChromeEvidence(pages []crawlPage, results []pageFactsResult) ([]pageFactsResult, []droppedFinding) {
	trimmed, blocks := stripSharedPrefixBlocks(pages)
	if len(blocks) == 0 {
		return results, nil
	}
	normalized := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if n := normalizeEvidence(block); n != "" {
			normalized = append(normalized, n)
		}
	}
	if len(normalized) == 0 {
		return results, nil
	}
	// What each page still says once its furniture is off. A snippet found in
	// the chrome AND in the page's own words is the page's own words: menus
	// repeat the site's vocabulary, so "Contact Us" or a company's name appears
	// in both, and dropping on the chrome match alone would take real evidence
	// with it. Over-trimming is the failure this whole lane is written to avoid.
	own := make(map[string]string, len(trimmed))
	for _, page := range trimmed {
		own[page.URL] = normalizeEvidence(page.Text)
	}

	var dropped []droppedFinding
	kept := make([]pageFactsResult, len(results))
	for i, result := range results {
		out := result
		prose := own[result.url]
		out.facts = nil
		for _, fact := range result.facts {
			if isChromeEvidence(fact.EvidenceSnippet, normalized, prose) {
				dropped = append(dropped, droppedFinding{
					Lane: chromeEvidenceLane, Field: fact.Field, Value: fact.Value,
					EvidenceSnippet: fact.EvidenceSnippet,
					Reason:          "the only passage citing it is the site's navigation, which every page carries",
				})
				continue
			}
			out.facts = append(out.facts, fact)
		}
		out.people = nil
		for _, person := range result.people {
			if isChromeEvidence(person.EvidenceSnippet, normalized, prose) {
				dropped = append(dropped, droppedFinding{
					Lane: chromeEvidenceLane, Field: chromeEvidencePersonField, Value: person.Name,
					EvidenceSnippet: person.EvidenceSnippet,
					Reason:          "the only passage naming them is the site's navigation, which every page carries",
				})
				continue
			}
			out.people = append(out.people, person)
		}
		out.entities = nil
		for _, entity := range result.entities {
			if isChromeEvidence(entity.EvidenceSnippet, normalized, prose) {
				dropped = append(dropped, droppedFinding{
					Lane: chromeEvidenceLane, Field: chromeEvidenceEntityField, Value: entity.Name,
					EvidenceSnippet: entity.EvidenceSnippet,
					Reason:          "the only passage naming it is the site's navigation, which names every candidate equally",
				})
				continue
			}
			out.entities = append(out.entities, entity)
		}
		kept[i] = out
	}
	return kept, dropped
}

// isChromeEvidence reports whether this snippet is furniture and ONLY
// furniture: inside a chrome block, and absent from what the page itself still
// says once that block is off.
//
// Both halves are load-bearing. A menu repeats the site's own vocabulary, so
// text can sit in the navigation AND in the prose — "Contact Us", a company's
// name, a product's — and a finding cited to the page's own words is a finding
// however many times the header also says them. Dropping on the chrome match
// alone would take real evidence with it, which is the over-trimming this lane
// is written to avoid: a missing fact is harder to notice than a noisy one.
//
// Containment, not equality: the model cites a PASSAGE, and the passage
// segmentation may cut the menu into several — so a snippet is chrome when the
// block contains it, however much of the block it happens to be.
//
// An empty snippet is not chrome. A finding with no evidence at all is the
// citation gate's business, and answering it here would make this the reason a
// reader is told when it was not.
func isChromeEvidence(snippet string, blocks []string, prose string) bool {
	normalized := normalizeEvidence(snippet)
	if normalized == "" {
		return false
	}
	inChrome := false
	for _, block := range blocks {
		if strings.Contains(block, normalized) {
			inChrome = true
			break
		}
	}
	return inChrome && !strings.Contains(prose, normalized)
}
