// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the crawled site DECLARES about its own stack.
//
// The technical lookup beside this one reads DNS and the certificate log, both
// of which answer about a DOMAIN. What software a site runs is answered by the
// site's own pages, and this read has already fetched them — every page it
// crawled carries the response headers, cookie names, script srcs and markup it
// arrived with (webread.Page.Fingerprint).
//
// Every page, not the homepage alone. A shop system announces itself on /shop,
// a portal on /kunden, a careers platform on /karriere, and a homepage-only
// fetch sees none of the three. This lane matches the same ruleset against
// everything the crawl reached, so the read that already paid for those pages
// is what answers the question.
//
// It owns the `homepage` lane on the record. The enricher no longer fetches a
// homepage of its own precisely so there is ONE writer: a completed lane
// reconciles, and two writers each seeing a different subset would take turns
// deleting the other's rows.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/techprofile"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readSiteTechnology matches every crawled page against the fingerprint
// ruleset and writes what they declared.
//
// Errors are logged and dropped, never returned. The dossier this read is
// about to close is complete without the technology rows, and a company that
// publishes no recognisable marker is the ordinary case rather than a fault.
//
// A read that resolved no organization writes nothing: the domain-triage lane
// runs before an account exists, and reading a site to decide whether to CREATE
// a company cannot enrich one.
func (w *siteDeepReadWorker) readSiteTechnology(ctx context.Context, claim people.SiteReadClaim, crawl siteCrawl) {
	if claim.OrganizationID == nil {
		return
	}
	orgID := ids.From[ids.OrganizationKind](*claim.OrganizationID)
	found, err := technologiesAcross(crawl.Pages)
	if err != nil {
		w.log.WarnContext(ctx, "site technology read failed",
			"organization", orgID.String(), "err", err)
		return
	}
	// The lane COMPLETED even when it found nothing: a site that declares no
	// recognisable stack is an authoritative empty answer, and saying so is
	// what lets a technology the company dropped leave the record.
	apply := people.TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []people.TechnicalLane{people.LaneHomepage},
		Observations:   found,
		ObservedAt:     time.Now().UTC(),
	}
	if err := w.people.ApplyTechnicalEnrichment(ctx, apply, technicalChangeRecorder()); err != nil {
		w.log.WarnContext(ctx, "writing what the site runs failed",
			"organization", orgID.String(), "err", err)
	}
}

// technologiesAcross matches the ruleset against every crawled page and returns
// one observation per technology.
//
// First page wins for a given technology, so the evidence cites the page that
// actually proved it rather than the last one to mention it — a reader asking
// "how do you know they run Shopware?" gets /shop, not whichever page sorted
// last. The crawl's own order is seed-first, so a marker on the homepage still
// cites the homepage.
func technologiesAcross(pages []crawlPage) ([]people.TechnicalObservation, error) {
	seen := map[string]bool{}
	observations := make([]people.TechnicalObservation, 0, len(pages))
	for _, page := range pages {
		fingerprint := page.Fingerprint
		if fingerprint.URL == "" {
			// A page the crawl recorded without a fetched response: the
			// certification fixtures build these, and a zero fingerprint
			// matches nothing anyway.
			continue
		}
		found, err := techprofile.Technologies(techprofile.Evidence{
			Headers:     fingerprint.Headers,
			CookieNames: fingerprint.CookieNames,
			ScriptSrcs:  fingerprint.ScriptSrcs,
			Generator:   fingerprint.Generator,
			Body:        fingerprint.Body,
		})
		if err != nil {
			return nil, err
		}
		for _, signal := range found {
			if seen[signal.Key] {
				continue
			}
			seen[signal.Key] = true
			observations = append(observations, observationOf(signal, fingerprint.URL))
		}
	}
	return observations, nil
}
