// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// siteAnotherBusinessWarning is the dossier caveat of a read that applied
// nothing because the site names somebody else.
const siteAnotherBusinessWarning = "This site names a different business than this company, so nothing it says was applied. The domain may have changed hands."

// siteSpeaksForCompany asks contacts.SiteSpeaksFor whether the site still
// belongs to the company it was read for. A read with no company yet has
// nothing to compare against.
func (w *siteDeepReadWorker) siteSpeaksForCompany(ctx context.Context, claim contacts.SiteReadClaim, extraction siteExtraction) (bool, error) {
	if claim.CompanyID == nil {
		return true, nil
	}
	stated := contacts.SiteStatedNames(deepReadFields(extraction.fields))
	for _, entity := range extraction.merged.entities {
		stated = append(stated, entity.Name)
	}
	speaks, err := w.contacts.SiteSpeaksForCompany(ctx, ids.From[ids.CompanyKind](*claim.CompanyID), stated)
	if err != nil {
		return false, fmt.Errorf("checking whose site this is: %w", err)
	}
	return speaks, nil
}

// readSideLanes runs what a read takes from a site beside its findings. A side
// lane's failure is never the read's. They run only for a site that speaks for
// the company.
func (w *siteDeepReadWorker) readSideLanes(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, crawl siteCrawl) {
	// The logo is a 🟢 display asset (A55) read off the seed page's markup, so it
	// lands even when the extraction comes back empty.
	w.resolveLogo(ctx, args, claim, crawl)
	w.readNewsroom(ctx, claim, crawl)
	w.readSiteTechnology(ctx, claim, crawl)
	w.askWhatTheCompanyRuns(ctx, claim)
}

// closeForeignRead settles a read whose site names another business. Nothing
// lands; the auto-enrich cursor records an empty outcome, because reading the
// same site again would find the same stranger.
func (w *siteDeepReadWorker) closeForeignRead(ctx context.Context, claim contacts.SiteReadClaim) {
	if !isAutoEnrichRequest(claim.RequestedBy) {
		return
	}
	companyID := ids.From[ids.CompanyKind](*claim.CompanyID)
	if err := w.autoEnrich.MarkResolved(ctx, companyID, autoEnrichOutcomeEmpty); err != nil {
		w.log.WarnContext(ctx, "auto-enrich cursor not recorded", "company", companyID.String(), "outcome", autoEnrichOutcomeEmpty, "err", err)
	}
}
