// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read's contact lane: a crawled team
// page spends its per-page category budget on CONTACTS — who the site
// itself publishes, and nothing more. The gate is stricter than the fact
// gate because this is the NEVER-8 boundary (thin, published-only): a
// contact survives only when name AND role are verbatim on the page, and a
// published_email / linkedin_url is kept only when the page prints it
// verbatim — otherwise the contact detail is stripped while the contact
// survives. Nothing is fabricated, nothing enriched from elsewhere.
// Contact pages keep their company category call and get NO contacts call:
// one call per page, and a contact page's deliberate content is the
// company's own contact identity, not a roster.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// siteLeadProposalKind is the staged per-contact proposal's wire identity —
// one spelling for the staging worker and the accept executor
// (siteleadaccept.go). One 🟡 per contact: each is decided on its own.
const siteLeadProposalKind = "site_lead"

// siteLeadProposal is the thin staged payload — exactly what the site
// published, plus the provenance the accept effect and the inbox need.
type siteLeadProposal struct {
	CompanyID  ids.UUID `json:"company_id"`
	SiteReadID ids.UUID `json:"site_read_id"`
	// NaturalKey is siteLeadSourceID for this contact — the SAME key the accept
	// effect captures the lead under, carried in the payload because the
	// approval's logical identity has to be a field the payload contains
	// (approvals.canonicalIdentity). It is what makes a re-read supersede the
	// undecided proposal from the last read instead of stacking beside it, and
	// it is normalized, so a page that reflows the name to "  anna   MUSTER "
	// still resolves to one question.
	NaturalKey      string `json:"natural_key"`
	Name            string `json:"name"`
	Role            string `json:"role"`
	PublishedEmail  string `json:"published_email,omitempty"`
	LinkedinURL     string `json:"linkedin_url,omitempty"`
	EvidenceSnippet string `json:"evidence_snippet"`
	SourceURL       string `json:"source_url"`
}

// siteContact is one gate-surviving published contact from a team page.
// Confidence stays extraction-internal (it ranks the cross-page merge);
// the staged payload carries only what the site published.
type siteContact struct {
	Name            string
	Role            string
	PublishedEmail  string
	LinkedinURL     string
	EvidenceSnippet string
	SourceURL       string
	Confidence      float32
}

// verbatimOrEmpty keeps a claimed contact detail only when the page text
// itself prints it — the site published it, so relaying it stays inside
// the published-only boundary. Anything else is dropped, never repaired.
func verbatimOrEmpty(claimed, pageText string) string {
	claimed = strings.TrimSpace(claimed)
	if claimed == "" || !strings.Contains(pageText, claimed) {
		return ""
	}
	return claimed
}

// siteContactIdentity is WHO a published contact is, for every step of a site
// read that has to decide whether two claims are one contact: the page lane's
// per-page fold, the cross-page merge, and the cross-read lead key below.
//
// The name goes through the module's key and not a local casefold: this is a
// contact's identity, so it is the same question contacts asks when it decides
// whether a captured contact is somebody already stored. That key UNACCENTS,
// which is what makes "José Silva" and "Jose Silva" one contact — and is also
// why the printed address is part of the identity rather than a late
// tie-break. Two real colleagues whose names differ only by an accent print
// two addresses, and a fold that stopped at the name would keep one of them
// and drop the other before the address was ever consulted.
func siteContactIdentity(name, publishedEmail string) string {
	key := contacts.NormalizeContactName(name)
	if e := strings.ToLower(strings.TrimSpace(publishedEmail)); e != "" {
		key += "|" + e
	}
	return key
}

// siteLeadSourceID is the lead's idempotency key under source_system
// "siteread": the COMPANY plus siteContactIdentity. Keyed on the company, not
// the page URL, so the same contact is the same lead whether they were found on
// /team or /about, and whether a later crawl's page layout moved — a page-URL
// key would duplicate them.
func siteLeadSourceID(companyID ids.UUID, name, publishedEmail string) string {
	digest := sha256.Sum256([]byte(companyID.String() + "|" + siteContactIdentity(name, publishedEmail)))
	return hex.EncodeToString(digest[:])
}
