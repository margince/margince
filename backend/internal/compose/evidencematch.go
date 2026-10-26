// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The evidence-matching relaxation under every no-guess gate. The gates
// demand a snippet the page actually carries; a model that faithfully
// quotes but normalizes typography — a curly quote straightened, an
// NBSP collapsed, a reflowed line — used to lose real facts to a
// byte-level strings.Contains. Matching now falls back to a normalized
// comparison that forgives ONLY presentation (case, whitespace, quote
// and dash glyphs, soft hyphens), never words: an invented or reworded
// snippet still fails, so the no-guess property stands.

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/evidencefold"
)

// evidenceOnPage answers whether the claimed snippet is on the page:
// the byte-exact fast path first, then the normalized comparison.
// pageNorm is normalizeEvidence(pageText), hoisted by the caller so one
// page's gate normalizes the page once, not per claimed field.
func evidenceOnPage(pageText, pageNorm, snippet string) bool {
	if strings.Contains(pageText, snippet) {
		return true
	}
	return strings.Contains(pageNorm, normalizeEvidence(snippet))
}

// normalizeEvidence is the comparison form every no-guess gate matches
// against; evidencefold owns it so the contacts module folds a name the same way.
func normalizeEvidence(s string) string {
	return evidencefold.Normalize(s)
}

// Lane names: which extraction a finding (or drop) belongs to. The
// category lanes are "category:<name>", built where the category is
// known.
const (
	laneFields    = "fields"
	laneContacts  = "contacts"
	lanePageFacts = "page_facts"
	laneProfile   = "profile"
	laneLegal     = "legal"
	laneTriage    = "triage"
)

// Drop reasons: why a model-claimed finding did not survive its gate.
// One vocabulary across the three lanes, so the debug report and the
// logs speak the same language.
const (
	dropUnknownField      = "unknown_field"
	dropDuplicate         = "duplicate"
	dropEmptyValue        = "empty_value"
	dropEmptyEvidence     = "empty_evidence"
	dropEvidenceNotOnPage = "evidence_not_on_page"
	dropConfidenceRange   = "confidence_out_of_range"
	dropBelowFloor        = "confidence_below_floor"
	// dropNameUnmappable marks a signature name that is missing a half or whose
	// parts do not read back as the printed line in the reported order. Nothing
	// is written then, because the other choice is a guess.
	dropNameUnmappable   = "name_not_mappable"
	dropNameRoleUnlinked = "name_role_not_in_snippet"
	dropNoPublishedEmail = "no_published_email"
	// dropEmailOffSiteDomain marks a published contact whose printed address
	// belongs to somebody else's domain — the testimonial case, where the page
	// prints a customer's own address and the read would file them as staff.
	dropEmailOffSiteDomain = "email_off_site_domain"
	// dropAlreadyOnFile marks a published contact the workspace already
	// holds — a contact who has been emailing us for months does not
	// become a decision because a crawler found their name.
	dropAlreadyOnFile = "already_on_file"
	// dropNoArticle14Notice marks published contacts not proposed while the
	// lane is shut (siteLeadLaneRefusal). It opens once their notice has passed
	// legal review.
	dropNoArticle14Notice = "no_article14_notice_path"
	dropEmptyValueKey     = "empty_value_key"
	dropZeroedStat        = "zeroed_stat"
	dropUnparseableReply  = "unparseable_reply"
	// dropLegalConflict marks a legal-trio claim refused because the
	// site's legal pages disagree on the entity: with no trustworthy
	// override, no lane may smuggle one back in.
	dropLegalConflict = "legal_conflict_no_override"
	// dropLegalCensusIncomplete marks the legal trio withheld because a
	// LEGAL page's fact call failed: with an entity vote possibly
	// missing, the abstention cannot trust its count.
	dropLegalCensusIncomplete = "legal_census_incomplete"
	// dropLegalNotFromLegalPage marks a legal-identity claim quoted from
	// anything but a shallow legal page — marketing copy cannot testify
	// to the register.
	dropLegalNotFromLegalPage = "legal_field_not_from_legal_page"
	// dropSnippetIDUnknown marks a citation outside the numbered index —
	// unreachable when the schema enum held, the honest gate when a
	// provider ignored it.
	dropSnippetIDUnknown = "snippet_id_unknown"
	// dropValueNotInSnippet marks a finding whose NAME the cited passage
	// (±1 same-page neighbor) does not carry — the reference-evidence
	// no-guess rule.
	dropValueNotInSnippet = "value_not_in_snippet"
	// dropLegalBlockNotThisEntity marks a legal detail refused because the
	// passage it was cited from never names the entity it is being
	// attributed to. Both halves are grounded on the page and neither is
	// invented, which is why this needs its own reason: the defect is the
	// ATTRIBUTION, and reporting it as an ungrounded value would send a
	// reader looking for text that is right there.
	dropLegalBlockNotThisEntity = "legal_block_not_this_entity"
	// dropSignatureNotThisContact marks a signature field refused because the
	// block it was read from names somebody else. The candidate query already
	// requires this contact to have SENT the message, so this catches the other
	// shape: their own mail carrying a colleague's or a correspondent's
	// signature below theirs, quoted or forwarded. Its own reason for
	// dropLegalBlockNotThisEntity's reason — the value is verbatim in the
	// window, so reporting it as ungrounded would send a reader hunting for
	// text that is plainly there.
	dropSignatureNotThisContact = "signature_not_this_contact"
	// dropParaphraseLowOverlap is WARNING-class, never a refusal: a
	// paraphrase profile field whose value shares no content word with
	// its cited passage. Multilingual sites trip it legitimately; the
	// rate is watched, the finding kept.
	dropParaphraseLowOverlap = "paraphrase_low_overlap"
)

// droppedFinding is one gate rejection, kept for the drop sink instead
// of vanishing: what the model claimed, and why it was refused.
type droppedFinding struct {
	Lane            string
	Field           string
	Value           string
	EvidenceSnippet string
	Reason          string
}
