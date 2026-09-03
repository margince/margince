// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read's category-fact vocabulary plumbing: the closed
// vocabulary lives with the people module (organization_fact's owner);
// this file carries the per-category prompt guidance the corpus call
// embeds and the fact dedupe identity the gate and the merges share.

import (
	"github.com/margince/margince/backend/internal/modules/people"
)

// The extraction envelope's JSON keys and the chat role — the same
// spellings the company-fact extraction (enrichextract.go) uses, named so
// the schema builder and the request can share them.
const (
	extractionEnvelopeKey   = "fields"
	extractionFieldKey      = "field"
	extractionValueKey      = "value"
	extractionEvidenceKey   = "evidence_snippet"
	extractionConfidenceKey = "confidence"
	chatRoleUser            = "user"
)

// categoryGuidance is the per-category slice of the extraction prompt:
// what the fields mean and, for the multi-value categories, the
// "Name — short description" value spelling NormalizeFactValueKey
// dedupes on.
var categoryGuidance = map[string]string{
	companyWord: "founded_year is the year the company was founded; " +
		"employee_range is THIS company's OWN headcount, exactly as printed — \"400 Mitarbeiter\", " +
		"\"800+ employees\", \"rund 130\", \"team of 20\". Take it wherever it appears, including a " +
		"homepage key-figures strip or an about page, and record the phrase rather than a rounded band. " +
		"It is NOT company_size, which is a market fact about the size of company they SELL TO, and it is " +
		"not a count of partners, customers, offices or locations — 550+ enterprises and around 500 partners " +
		"are neither. " +
		"phone and contact_email the company's own contact details; location one entry per office or site the " +
		"company states (city and country as printed).",
	"offering": "service and product name what THIS company sells, at the level it sells them — the page's own subject, as a buyer would name it on an order. " +
		"A product is software or a repeatable packaged good the buyer uses; a service is work this company performs for the buyer. Use capability when the page states an ability but does not sell it as a named offer. " +
		"A method, technique, step, phase or deliverable USED TO DELIVER one offering is not itself an offering: on a page about a research service, " +
		"the service is what the page is about, while workshops, interviews, mapping and synthesis are how it is done — omit those. " +
		"A product, platform or vendor made by SOMEONE ELSE that this company integrates, migrates, partners on or builds upon is technology, NEVER product or service, " +
		"however deeply the page describes working with it. " +
		"capability names a delivery or technical capability the company declares about ITSELF — what it can do for any client — never an implementation detail, " +
		"configuration, or feature bullet of one project, page or engagement. One entry per item, repeating the field name. " +
		"A case study, testimonial or customer story page's subject is the NAMED CUSTOMER, not this company: a product, service or capability the story credits to that " +
		"customer's own business — what THEY sell, run or offer their own buyers — is a fact about the customer, never this company's offering, however the page frames the " +
		"story or however much of the page it fills. Extract only what the page states THIS company sells, never what a featured customer sells — but the company's OWN " +
		"product or service the story says the customer adopted, bought or uses is still this company's offering and belongs in the answer exactly as it would on any other page.",
	"market": "served_industry, company_size, geography and language describe markets the company explicitly says it serves — one entry per grounded item, repeating the field name. " +
		"company_size here is the size of the customers they sell TO (\"we work with mid-sized retailers\"), never their own headcount, which is employee_range under company. " +
		"The subject decides it, not the wording: the passage must be in THIS company's own voice about ITS OWN buyers (\"we work with mid-sized retailers,\" \"our clients include\") " +
		"— never a sentence that happens to use similar words while describing a NAMED CUSTOMER's own business. A case study, testimonial or customer story names a customer's own " +
		"industry, headquarters, geography, size or language — where THAT company is based, how big it is, what industry it is in, who IT serves, what language ITS OWN site or " +
		"materials are in — and that is a fact about the customer telling their own story, never a market this company serves, however closely the sentence reads like one. Omit " +
		"any served_industry, geography, company_size or language stated about the customer inside their own case study rather than stated about this company's own customer base.",
	"signal": "certification names a held certification or standard; partner a named business partner; " +
		"named_customer a customer the site names; " +
		"technology a named platform, product or stack the company states it USES, RUNS or BUILDS IN — its own " +
		"stack, not its subject matter. The passage must assert this company's own use: \"built on X\", \"we run X\", " +
		"\"our X-based platform\", \"migrating our shop to X\". A vendor merely NAMED, described, compared, offered as " +
		"an integration or listed among options is not a technology fact, however much text the page spends on it — " +
		"a page selling integrations names every vendor it integrates with, and none of them is a statement about " +
		"what this company runs. NEVER an analyst firm, rating, report or award (Gartner, Forrester, a Magic " +
		"Quadrant, a Wave) — those rate a company, they are not something it uses. NEVER a bare capability " +
		"category (BI, CRM, ERP, PIM, e-commerce, cloud): a category is not a product. If the page does not say " +
		"THIS company uses it, omit it. " +
		"quantified_outcome preserves an exact measurable customer or case-study result " +
		"without strengthening the claim — one entry per item, repeating the field name.",
}

// factKey is a fact's dedupe identity — the columns of uq_org_fact minus
// the tenant and the org, both fixed within one read.
func factKey(f people.DeepReadFact) string {
	return f.Category + "\x00" + f.Field + "\x00" + f.ValueKey
}
