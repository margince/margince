// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// RFC 8058 one-click unsubscribe wiring (B-E11.32, features/06 §1.2
// AC-D3): a bulk/marketing send carries a machine-readable
// List-Unsubscribe URL plus the List-Unsubscribe-Post one-click marker,
// and a visible link in the body. The URL points at the preference
// center's public POST endpoint, whose token the consent module mints per
// recipient. activities never imports consent — the composition root
// injects the linker.

import (
	"context"
	"html"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// UnsubscribeLinker resolves a recipient address to their preference-center
// token so the send path can build the List-Unsubscribe URL. ok is false
// when the address carries no unsubscribe surface — a locked
// (transactional) purpose, or an address no contact holds — in which case
// the send carries no unsubscribe header.
type UnsubscribeLinker interface {
	// UnsubscribeToken answers the credential the STOP links carry, and ok is
	// false when this address has no unsubscribe surface at all. purpose is
	// the key that credential stops, empty when it stops all marketing.
	UnsubscribeToken(ctx context.Context, recipientEmail, purposeKey string) (token, purpose string, ok bool, err error)
	// ManageToken answers the credential the preference centre resolves, which
	// is a different capability and now a different kind of token.
	//
	// A SEPARATE METHOD rather than a second return value, because the two
	// have different failure meanings. No stop token means this send carries no
	// unsubscribe surface and the header is omitted. No manage token means the
	// recipient has nothing to manage — a lead-only address holds no contact
	// record — and the send still goes out with its stop links intact.
	ManageToken(ctx context.Context, recipientEmail string) (token string, ok bool, err error)
}

// WithUnsubscribe wires the RFC 8058 linker onto the send path. A send
// composed without one simply carries no unsubscribe header — a marketing
// send still requires granted consent at the gate, so the missing header
// is a wiring gap, never a suppression bypass.
func (h Handlers) WithUnsubscribe(linker UnsubscribeLinker) Handlers {
	h.store = h.store.WithUnsubscribe(linker)
	return h
}

// WithUnsubscribe is the store-level wiring the handler option delegates to:
// the derivation belongs to the send path, which the MCP tool surface enters
// without passing through any handler.
func (s *Store) WithUnsubscribe(linker UnsubscribeLinker) *Store {
	clone := *s
	clone.unsubscribe = linker
	return &clone
}

// WithPublicBaseURL sets the canonical scheme+host the recipient's
// unsubscribe/preference links resolve to. It is configured at boot, NEVER
// derived from the inbound request: the link carries the recipient's
// unsubscribe token, so trusting a request Host/X-Forwarded-Proto header
// would let an attacker who controls it at send time point the tokenized
// link at their own domain and harvest the token.
func (h Handlers) WithPublicBaseURL(base string) Handlers {
	h.store = h.store.WithPublicBaseURL(base)
	return h
}

// WithPublicBaseURL is the store-level half of the same wiring; it also
// supplies the domain every minted Message-ID is qualified by.
func (s *Store) WithPublicBaseURL(base string) *Store {
	clone := *s
	clone.publicBaseURL = strings.TrimRight(strings.TrimSpace(base), "/")
	return &clone
}

// redactedToken stands in for the recipient's preference token in the copy of
// the message the workspace RECORDS. The token is a bearer credential over
// that contact's consent record — on the anonymous public edge it reads their
// per-purpose state, withdraws, and grants, under a system principal that
// short-circuits every RBAC gate — so it belongs on the mail and nowhere
// else: not in the durable activity body, which any seat holding
// activity:read serves back (the seeded read_only role reads every one of
// them), and not in the 202 the API caller reads.
//
// The record keeps the footer's SHAPE, built by the same two functions from
// this stand-in, so a reader still sees that the send carried a working
// one-click link and which purpose it pointed at, and loses only the value
// that would let them use it. It is path-safe on purpose: url.PathEscape
// leaves it alone, so the recorded URL reads as a URL and can never be
// mistaken for the pref_-prefixed credential it replaces.
const redactedToken = "token-redacted"

// sendDeliverability is what a send must carry for a mailbox provider to
// accept it as bulk mail, in the three renderings that must not be confused
// for one another.
type sendDeliverability struct {
	footerFindings []commsauthz.FooterFinding
	// listUnsubscribe is the RFC 8058 header value.
	listUnsubscribe string
	// disclosures are the obligations this message carries, kept so the MARKUP
	// alternative renders the same ones. Two alternatives of one message that
	// disagreed about what was disclosed would be two messages, and which one
	// the recipient reads is their client's decision rather than ours.
	disclosures []DisclosureLine
	// transmitted is the body that goes on the wire. It carries the live
	// token, because the recipient's one-click link IS that token.
	transmitted string
	// recorded is the body the timeline row keeps and every authenticated
	// read of it serves: the same message with the capability redacted.
	recorded string
	// links are the three destinations this send offers, kept so the markup
	// alternative renders anchors from the SAME token. Rebuilding them from a
	// second token would let the two parts of one message offer different
	// unsubscribe capabilities.
	links unsubscribeLinks
	// words are the footer's labels in the message's own language.
	words mailcopy.Copy
}

// unsubscribeLinks is every destination one send derives from ONE token.
//
// Held by: TestOneSendDerivesEveryUnsubscribeDestination (backend/gates/preferencecentrewriters_test.go)
//
// Three, not one, because three different callers press them and they need
// different shapes. Collapsing them is what broke: the visible "Unsubscribe"
// link pointed at the machine endpoint, which is POST-only by design, so a
// human clicking it in a mail client got 405 — and the visible "Manage
// preferences" link pointed at the JSON API, which answered a recipient with
// a wall of JSON. Built by one function so the three cannot name different
// tokens, purposes or languages.
type unsubscribeLinks struct {
	// oneClick is the RFC 8058 endpoint, for the List-Unsubscribe header and
	// NOTHING else. A mailbox provider POSTs it without a browser.
	oneClick string
	// unsubscribe is the page a contact lands on, which asks before it acts:
	// a GET must never withdraw, because mail scanners and link prefetchers
	// follow links in a mailbox without a human involved.
	unsubscribe string
	// manage is the preference centre, where every purpose can be seen.
	manage string
}

// unsubscribeLinksFor builds all three from one token.
//
// The two visible links are hash routes: the SPA already serves its public
// surfaces that way, static hosting needs no server fallback for them, and
// the token stays out of ordinary web-server access logs until the page
// deliberately calls the API with it.
//
// An empty purpose names no purpose anywhere: the press then stops all
// marketing, which is the scope the credential was minted with.
func unsubscribeLinksFor(baseURL string, tokens unsubscribeTokens, purposeKey string, lang textlang.Lang) unsubscribeLinks {
	purpose := commsauthz.NormalizedPurposeKey(purposeKey)
	query := "?lang=" + url.QueryEscape(string(lang))
	manageToken := tokens.manage
	if manageToken == "" {
		manageToken = tokens.stop
	}
	oneClick := baseURL + "/v1/public/preferences/" + url.PathEscape(tokens.stop) + "/unsubscribe"
	page := baseURL + "/#/unsubscribe/" + url.PathEscape(tokens.stop)
	if purpose != "" {
		oneClick += "?purpose=" + url.QueryEscape(purpose)
		page += "/" + url.PathEscape(purpose)
	}
	return unsubscribeLinks{
		oneClick:    oneClick,
		unsubscribe: page + query,
		manage:      baseURL + "/#/preferences/" + url.PathEscape(manageToken) + query,
	}
}

// unsubscribeTokens is the two capabilities one send hands a recipient.
//
// TWO, because stopping mail and reading a record are different rights and the
// credentials for them now differ in kind. The stop token is a withdrawal
// credential: long-lived on purpose, so the link in a message somebody kept
// still works months later, and it reads nothing. The manage token is a
// preference token, which is what the preference centre resolves — it slides
// and is rotated by the next send, and it can show the subject their own
// purposes.
//
// Passing one token for both is what broke the manage page: every link carried
// the withdrawal credential, the preference centre resolved nothing, and a
// recipient clicking "Manage preferences" got a page that could not read them.
// The visible unsubscribe page degrades to withdraw-only in that case, which is
// still honest; the manage page has nothing to degrade to.
type unsubscribeTokens struct {
	// stop is what the one-click endpoint and the human unsubscribe page carry.
	// It must outlive the message it was sent in.
	stop string
	// manage is what the preference centre resolves. Empty when this send could
	// mint none — a lead-only recipient holds no contact record to manage — and
	// the manage link then falls back to the stop token, which draws the
	// withdraw-only page rather than a dead link.
	manage string
}

// htmlFooter renders the unsubscribe links as markup, for the alternative part.
// Empty when this send carries no unsubscribe surface, which is what a
// transactional purpose has: nothing to unsubscribe from and no footer.
func (d sendDeliverability) htmlFooter() string {
	if d.links.unsubscribe == "" {
		return ""
	}
	return `<hr><p><a href="` + html.EscapeString(d.links.unsubscribe) + `">` +
		html.EscapeString(d.words.UnsubscribeLabel) + `</a>` +
		` · <a href="` + html.EscapeString(d.links.manage) + `">` +
		html.EscapeString(d.words.ManagePreferencesLabel) + `</a></p>`
}

// unsubscribeSurface says whether this message offers a way to stop, and which
// topic that offer is about.
//
// TWO fields because they answer two questions and the old single purpose key
// conflated them: whether an unsubscribe surface belongs on this message at all
// (the category), and which consent purpose the link should point at (the
// marketing topic). A marketing send names its topic in MarketingPurpose; the
// deprecated ConsentPurpose is still read as a fallback so an older caller that
// names only a key keeps the surface it had.
type unsubscribeSurface struct {
	category   commsauthz.Category
	purposeKey string
}

// surfaceFor reads the unsubscribe surface off a send's own fields.
//
// The topic wins over the deprecated key, and both are read: a marketing send
// names its topic in MarketingPurpose, while an older caller that names only a
// consent key keeps pointing the link where it always did.
func surfaceFor(category commsauthz.Category, marketingPurpose, consentPurpose string) unsubscribeSurface {
	key := marketingPurpose
	if key == "" {
		key = consentPurpose
	}
	return unsubscribeSurface{category: category, purposeKey: key}
}

// carries reports whether this message offers an unsubscribe at all.
//
// A message with NO category falls back to the deprecated key, so a caller that
// predates the category — an MCP tool, a stored scheduled send — keeps the
// behaviour it was written against rather than silently losing or gaining a
// footer.
// disclosureCategory answers which category the DISCLOSURE rules should be
// resolved under.
//
// The category when there is one. When there is not — an older caller naming
// only the deprecated consent key — it falls back to marketing exactly when the
// legacy surface says this message carries an unsubscribe link, because that is
// the same question: a send offering somebody a way to stop receiving it is
// advertising by this product's own reckoning.
//
// It never answers marketing for a message with an explicit non-marketing
// category. The fallback exists for callers that said nothing, not to overrule
// one that did.
func (u unsubscribeSurface) disclosureCategory() commsauthz.Category {
	if u.category != "" {
		return u.category
	}
	if u.carries() {
		return commsauthz.CategoryMarketing
	}
	return ""
}

func (u unsubscribeSurface) carries() bool {
	if u.category != "" {
		return u.category.CarriesUnsubscribe()
	}
	return u.purposeKey != "" && !lockedPurposeKey(u.purposeKey)
}

func (u unsubscribeSurface) privateLinks() bool {
	return u.carries() && !lockedPurposeKey(u.purposeKey)
}

// lockedPurposeKey mirrors consent.LockedPurpose for the legacy arm above.
// activities may not import consent, and the composition root injects the
// linker rather than the rule, so the one key that has ever been locked is
// named here.
//
// Held by: TestTheLegacyLockedKeyAgreesWithConsent (backend/gates/unsubscribesurface_test.go)
func lockedPurposeKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "transactional")
}

// offersUnsubscribe is asked by delivery and the footer preview alike. It reads
// the category, not the purpose key. A reply carries no key, and must not offer
// a stop link for a conversation the recipient started.
func (s *Store) offersUnsubscribe(recipients []string, surface unsubscribeSurface) bool {
	return s.unsubscribe != nil && len(recipients) > 0 && distinctAddresses(recipients) == 1 &&
		s.publicOriginUsable() == nil && surface.privateLinks()
}

// deliverability renders usable disclosures and private links. Missing parts
// travel with the message for the sender's recorded confirmation.
func (s *Store) deliverability(
	ctx context.Context, body, subject string, recipients []string, countries []commsauthz.CountryOverride, surface unsubscribeSurface, entity ids.UUID, lang textlang.Lang,
) (sendDeliverability, error) {
	// THE SURFACE decides whether this is advertising, not the category alone.
	// A caller that names only the deprecated consent key carries no category,
	// and asking about an empty one drops every marketing-only obligation — the
	// German objection route and the Vietnamese advertiser contact — from
	// exactly the sends that carry an unsubscribe surface and are therefore
	// advertising by the product's own reckoning.
	if lang == "" {
		lang = s.footerLanguage(ctx, body, subject)
	}
	owed, err := s.disclosuresFor(ctx, string(surface.disclosureCategory()), recipients, countries, entity, string(lang))
	if err != nil {
		return sendDeliverability{}, err
	}
	body = appendDisclosures(body, owed)
	untokenized := sendDeliverability{
		transmitted: body, recorded: body, disclosures: owed,
		footerFindings: s.footerFindings(owed, recipients, surface.privateLinks()),
	}
	if !s.offersUnsubscribe(recipients, surface) {
		return untokenized, nil
	}
	token, purpose, ok, err := s.unsubscribe.UnsubscribeToken(ctx, recipients[0], surface.purposeKey)
	if err != nil {
		return sendDeliverability{}, err
	}
	if !ok {
		return untokenized, nil
	}

	words := mailcopy.For(string(lang))
	// Asked AFTER the stop token, and its failure is not the send's. A
	// recipient who cannot be given a preference centre can still be given a
	// way to stop, which is the link that matters.
	manageToken, _, err := s.unsubscribe.ManageToken(ctx, recipients[0])
	if err != nil {
		return sendDeliverability{}, err
	}
	live := unsubscribeLinksFor(s.publicBaseURL,
		unsubscribeTokens{stop: token, manage: manageToken}, purpose, lang)
	redacted := unsubscribeLinksFor(s.publicBaseURL,
		unsubscribeTokens{stop: redactedToken, manage: redactedToken}, purpose, lang)
	return sendDeliverability{
		footerFindings:  untokenized.footerFindings,
		listUnsubscribe: listUnsubscribeHeader(live.oneClick),
		links:           live,
		words:           words,
		disclosures:     owed,
		transmitted:     appendUnsubscribeFooter(body, live, words),
		recorded:        appendUnsubscribeFooter(body, redacted, words),
	}, nil
}

// distinctAddresses counts who a rendered message actually reaches. Addresses
// are compared case- and space-insensitively, the way a mail server treats
// them, so the same contact listed twice — once in To and once in Cc, or with
// different capitalisation — is one addressee and not a refusal.
func distinctAddresses(recipients []string) int {
	seen := make(map[string]bool, len(recipients))
	for _, addr := range recipients {
		key := normalizeAddress(addr)
		if key == "" {
			continue
		}
		seen[key] = true
	}
	return len(seen)
}

// listUnsubscribeHeader returns the RFC 8058 List-Unsubscribe value: the
// bracketed https one-click URL. Its companion List-Unsubscribe-Post value
// is fixed by the RFC at "List-Unsubscribe=One-Click" and is therefore
// rendered at the wire from this value being present, never carried or
// stored beside it — one value cannot drift out of step with itself.
func listUnsubscribeHeader(unsubURL string) string {
	return "<" + unsubURL + ">"
}

// appendUnsubscribeFooter adds the human-visible unsubscribe + manage-
// preferences links beneath the message body (AC-D3 "a visible unsubscribe
// link"), built from the same token as the machine header so the two can
// never diverge. Both point at PAGES, never at the API: the machine
// endpoint is POST-only and answers a human click with 405.
func appendUnsubscribeFooter(body string, links unsubscribeLinks, words mailcopy.Copy) string {
	return body + "\n\n---\n" +
		words.UnsubscribeLabel + ": " + links.unsubscribe + "\n" +
		words.ManagePreferencesLabel + ": " + links.manage
}
