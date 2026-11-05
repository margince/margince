// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

func (w *siteDeepReadWorker) withholdForwardedTriage(ctx context.Context, said domainEvidenceCopy, args SiteDeepReadArgs, claim contacts.SiteReadClaim, domain, effectiveURL string, pages []crawlPage) error {
	chain := []string{claim.SeedURL, effectiveURL}
	if len(pages) > 0 && len(pages[0].RedirectChain) > 0 {
		chain = pages[0].RedirectChain
	}
	if err := w.contacts.WithholdForwardedDomain(ctx, contacts.DomainForwardEvidence{
		ReadID: args.SiteReadID, RequestedDomain: domain, EffectiveURL: effectiveURL, RedirectChain: chain,
	}); err != nil {
		return w.fail(ctx, args.SiteReadID, err)
	}
	return w.finishTriageRead(ctx, said, args, claim, siteReadWireStatusDone, "", &triagePayload{
		SeedURL: effectiveURL, Crawl: siteCrawl{SeedURL: effectiveURL, Pages: pages},
	})
}
