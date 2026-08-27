// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A company's own newsroom, as signals on its account.
//
// The whole of what is stored is the headline, the date and the address. The
// article's text is never cached: what a company published is theirs, and a CRM
// holding a copy of somebody's press page is the shape this product refuses
// everywhere else. A reader who wants the article follows the link.
//
// The classification reads the HEADLINE alone — no body, no page — and is
// deterministic rather than a model call. The four kinds are announcement
// genres with stable vocabulary in both languages this reads, so a word list
// answers the question, and a model would spend a round trip per item of every
// crawl to answer it less predictably. A headline that matches nothing is filed
// as `other`, because a funding round that was actually a hiring announcement
// is worse on an account page than an unclassified line.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// severityInfo is what a company's own announcement is: news about the
// account, never a problem with it. `warn` and `urgent` are what a reader
// triages by, and a press release competing there for attention would make
// that column mean less.
const severityInfo = "info"

// The four kinds a newsroom item can be, plus the one it is when the reader
// cannot tell. Spelled here because this file is what produces them.
const (
	kindFunding          = "funding"
	kindLeadershipChange = "leadership_change"
	kindExpansion        = "expansion"
	kindProductLaunch    = "product_launch"
	kindOtherEvent       = "other"
)

// newsroomChannel and newsroomSource say where these rows came from, in the
// signal table's own vocabulary and in the audit trail's.
const (
	newsroomChannel = "web"
	newsroomSource  = "site-newsroom"
)

// newsroomMaxAge bounds how far back a first read files. A newsroom lists its
// archive, and a company's whole press history arriving at once would bury the
// account's real work under announcements nobody is acting on.
const newsroomMaxAge = 365 * 24 * time.Hour

// NewsroomItem is one classified item, ready to file.
type NewsroomItem struct {
	Kind      string
	Headline  string
	URL       string
	Published time.Time
}

// WriteNewsroomSignals files what a company announced about itself.
//
// Returns how many were raised; an item already on file raises nothing, which
// is the normal case for a feed read twice and not a failure. The fingerprint
// is the item's own address, so a headline the CMS later rewords does not
// arrive as a second event.
func WriteNewsroomSignals(
	ctx context.Context,
	tx pgx.Tx,
	orgID ids.UUID,
	items []NewsroomItem,
	now time.Time,
) (int, error) {
	raised := 0
	for _, item := range items {
		if stale(item, now) {
			continue
		}
		filed, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind:           item.Kind,
			OrganizationID: orgID,
			Summary:        item.Headline,
			// Never `warn` or `urgent`: a company announcing something is news
			// about the account, not a problem with it, and the severity
			// vocabulary is what a reader triages by.
			Severity: severityInfo,
			Channel:  newsroomChannel,
			Source:   newsroomSource,
			// The article's ADDRESS is the identity, and the kind is
			// deliberately not in it: a headline the CMS rewords, or a rule
			// that later places it differently, is the same announcement and
			// must not arrive as a second event.
			Fingerprint: fingerprintOf(newsroomSource, orgID.String(), item.URL),
			// The article is CITED, never copied: the snippet is the headline
			// the company itself published, and the source is where to read the
			// rest.
			Evidence: []signals.DerivedEvidence{{
				Snippet:   item.Headline,
				SourceURL: item.URL,
			}},
			Audit: map[string]any{
				paramKind:      item.Kind,
				"source_url":   item.URL,
				"published_at": publishedForAudit(item.Published),
			},
		}, detectedAt(item, now))
		if err != nil {
			return raised, fmt.Errorf("filing a newsroom signal: %w", err)
		}
		if filed {
			raised++
		}
	}
	return raised, nil
}

// stale drops what is too old to be news. An item with no date at all is kept:
// the feed stated no date, which is not the same as stating an old one, and
// dropping it would silently lose a company's whole newsroom when its CMS
// omits the field.
func stale(item NewsroomItem, now time.Time) bool {
	if item.Published.IsZero() {
		return false
	}
	return now.Sub(item.Published) > newsroomMaxAge
}

// detectedAt dates the signal by the item's own publication, falling back to
// the read. A signal is triaged by when it happened, and the fallback is only
// for a feed that stated nothing.
func detectedAt(item NewsroomItem, now time.Time) time.Time {
	if item.Published.IsZero() {
		return now
	}
	return item.Published
}

// publishedForAudit renders the item's date for the audit row, or nil when the
// feed stated none.
//
// A *string rather than a bare any: nil marshals to JSON null either way, and
// the pointer says in the signature that absence is one of the two answers.
// Spelled as null rather than omitted, because "the feed gave no date" is a
// fact about the source and an absent key would read as a field the write
// forgot.
func publishedForAudit(published time.Time) *string {
	if published.IsZero() {
		return nil
	}
	stamped := published.UTC().Format(time.RFC3339)
	return &stamped
}

// classifyHeadline places one headline in the four-kind vocabulary.
//
// Deterministic and local: the kinds are announcement genres with stable
// vocabulary, and a model call per headline would cost a round trip for each
// item of every crawl to answer a question a word list answers. A headline that
// matches nothing is `other`, which is an honest verdict rather than a guess.
func classifyHeadline(headline string) string {
	lowered := strings.ToLower(headline)
	for _, rule := range headlineRules {
		for _, phrase := range rule.words {
			if containsPhrase(lowered, phrase) {
				return rule.kind
			}
		}
	}
	return kindOtherEvent
}

// containsPhrase matches on word boundaries rather than as a substring.
//
// A bare Contains claims every word the token merely sits inside: "praises"
// would answer to "raises", and a company praising its new CEO would be filed
// as having raised money. The phrases themselves may hold spaces ("series b"),
// so the test is on what surrounds the match rather than on a split.
func containsPhrase(haystack, phrase string) bool {
	from := 0
	for {
		at := strings.Index(haystack[from:], phrase)
		if at < 0 {
			return false
		}
		at += from
		before := at == 0 || !isWordByte(haystack[at-1])
		end := at + len(phrase)
		after := end == len(haystack) || !isWordByte(haystack[end])
		if before && after {
			return true
		}
		from = at + 1
	}
}

// isWordByte reports whether a byte continues a word. ASCII only, deliberately:
// what it guards against is a token sitting inside a longer ASCII word, and a
// multi-byte rune's bytes are all >= 0x80 and answer false — the right answer
// at a boundary like "übernimmt".
func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// headlineRules is read in order, so an earlier rule wins a headline both would
// claim. Funding leads because a funding announcement routinely names the
// growth it will pay for, and the money is the event.
var headlineRules = []struct {
	kind  string
	words []string
}{
	{kindFunding, []string{
		"series a", "series b", "series c", "series d", "seed round",
		"funding", "raises", "raised", "investment round", "finanzierung",
		"finanzierungsrunde", "kapitalerhöhung",
	}},
	{kindLeadershipChange, []string{
		"appoints", "appointed", "names new", "joins as", "steps down",
		"new ceo", "new cfo", "new cto", "new coo", "chief executive",
		"managing director", "ernennt", "neuer geschäftsführer", "vorstand",
	}},
	{kindExpansion, []string{
		"opens", "expands", "expansion", "new office", "acquires",
		"acquisition", "merger", "enters the", "eröffnet", "übernimmt",
		"expandiert", "neuer standort",
	}},
	{kindProductLaunch, []string{
		"launches", "launch of", "introduces", "unveils", "now available",
		"announces the release", "general availability", "startet",
		"stellt vor", "veröffentlicht",
	}},
}
