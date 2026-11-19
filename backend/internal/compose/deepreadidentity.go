// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// siteAnotherBusinessWarning is the dossier caveat of a read that applied
// nothing because the site names somebody else. It says how a colleague
// recovers when the company did rebrand or belongs to a parent brand.
const siteAnotherBusinessWarning = "This site names a different business than this company, so nothing it says was applied. " +
	"The domain may have changed hands. If the company now uses this name, add the name or the domain to the company and read the site again."

// gatedRead is a read's findings after the legal gate, with the answer to
// whose site it is.
type gatedRead struct {
	fields       []evidencedField
	legalWarning string
	speaks       bool
}

// siteIdentity says whether the site speaks for the company, and whether it
// named anybody at all. A site that names nobody may still apply findings, but
// its logo is nobody's to wear.
type siteIdentity struct {
	speaks bool
	named  bool
}

// siteIdentityOf asks contacts.SiteSpeaksFor of the names the gated read states,
// the same reading triage names a company from. When the model stated none, the
// seed page's own <head> name stands in. A read with no company yet has nothing
// to compare against.
func (w *siteDeepReadWorker) siteIdentityOf(ctx context.Context, claim contacts.SiteReadClaim, crawl siteCrawl, fields []evidencedField, entities []corpusLegalEntity) (siteIdentity, error) {
	if claim.CompanyID == nil {
		return siteIdentity{speaks: true, named: true}, nil
	}
	stated := contacts.SiteStatedNames(deepReadFields(fields))
	for _, entity := range entities {
		stated = append(stated, entity.Name)
	}
	if !namesAnybody(stated) {
		stated = siteNameSegments(crawl.SeedSiteName)
	}
	if !namesAnybody(stated) {
		return siteIdentity{speaks: true}, nil
	}
	speaks, err := w.contacts.SiteSpeaksForCompany(ctx, ids.From[ids.CompanyKind](*claim.CompanyID), stated)
	if err != nil {
		return siteIdentity{}, fmt.Errorf("checking whose site this is: %w", err)
	}
	return siteIdentity{speaks: speaks, named: true}, nil
}

func namesAnybody(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			return true
		}
	}
	return false
}

// siteNameSegments splits a <title> such as "Home | Acme" into the parts a
// site joins its page and its own name with.
func siteNameSegments(title string) []string {
	return strings.FieldsFunc(title, func(r rune) bool { return strings.ContainsRune("|–—:·", r) })
}

// readSideLanes runs what a read takes from a site beside its findings, only
// for a site that speaks for the company. A side lane's failure is never the
// read's.
func (w *siteDeepReadWorker) readSideLanes(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, crawl siteCrawl, identity siteIdentity) {
	if !identity.speaks {
		return
	}
	if identity.named {
		// The logo is a 🟢 display asset (A55) read off the seed page's markup.
		w.resolveLogo(ctx, args, claim, crawl)
	}
	w.readNewsroom(ctx, claim, crawl)
	w.readSiteTechnology(ctx, claim, crawl)
	w.askWhatTheCompanyRuns(ctx, claim)
}

// closeForeignRead settles an automatic read whose site names another business.
// Its cursor records `empty`, the outcome for a read that worked and landed
// nothing; `failed` would mark the company for a retry.
func (w *siteDeepReadWorker) closeForeignRead(ctx context.Context, claim contacts.SiteReadClaim) {
	if !isAutoEnrichRequest(claim.RequestedBy) {
		return
	}
	companyID := ids.From[ids.CompanyKind](*claim.CompanyID)
	if err := w.autoEnrich.MarkResolved(ctx, companyID, autoEnrichOutcomeEmpty); err != nil {
		w.log.WarnContext(ctx, "auto-enrich cursor not recorded", "company", companyID.String(), "outcome", autoEnrichOutcomeEmpty, "err", err)
	}
}
