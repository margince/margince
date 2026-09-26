// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// siteLeadCaptureOpen says whether a person a company's website names may become
// a lead. It stays false until accepting one also sends that person the Article 14
// notice and records that it was sent: named-person website enrichment does not
// switch on before that path exists, and capturing a stranger without it is the
// thing the promise rules out. Staging refuses while it is false, and so does
// accepting a proposal staged before it was. The step it waits for is a function
// named sendArticle14Notice, called directly in siteLeadAcceptEffect's body.
//
// Held by: TestSiteLeadCaptureOpensOnlyWithANoticeStep
// (backend/internal/compose/siteleadnotice_test.go), which fails in both
// directions: open with no notice step in the accept path, or a notice step
// built while the lane stays shut.
const siteLeadCaptureOpen = false

// errSiteLeadCaptureClosed is what accepting a site lead answers while the lane
// is shut. A conflict rather than a denial: the decider may accept proposals,
// and this one cannot be carried out until the notice path exists.
var errSiteLeadCaptureClosed = fmt.Errorf("%w: a person named on a website cannot be captured "+
	"until Margince can send them the Article 14 notice; the proposal stays in the inbox", apperrors.ErrConflict)

// siteLeadPrecheck refuses accepting a site lead before the decision commits,
// so a proposal staged while the lane was open stays pending in the inbox
// instead of turning into an approval nothing carries out.
func siteLeadPrecheck() approvals.ReleasePrecheck {
	return func(context.Context, json.RawMessage, json.RawMessage) error {
		if !siteLeadCaptureOpen {
			return errSiteLeadCaptureClosed
		}
		return nil
	}
}

// siteLeadsRefused is the one staging-side check, asked by every path that would
// stage website people: the crawl worker's and the onboarding confirmation's. It
// reports true, and says why in the log, when the lane is shut and there was
// somebody to refuse.
func siteLeadsRefused(ctx context.Context, log *slog.Logger, readID string, found int) bool {
	if siteLeadCaptureOpen || found == 0 {
		return false
	}
	log.InfoContext(ctx, "published contacts not proposed",
		"lane", laneContacts, "reason", dropNoArticle14Notice, "read", readID, "contacts", found)
	return true
}
