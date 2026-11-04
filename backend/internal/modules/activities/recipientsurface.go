// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// surfaceForRecipients is the send's surface, widened to advertising when the
// engine resolves any recipient's copy to marketing. One body goes to every
// recipient. So a reply to one contact, with an advertised contact on Cc,
// carries the objection route and the unsubscribe link.
//
// The engine is asked through the send's own gate, so the answer comes from
// the authority staging asks again. Staging refuses a body composed without
// these duties that a recipient then resolves to marketing
// (commsauthz.Request.OmitsAdvertisingDuties).
func surfaceForRecipients(ctx context.Context, in SendEmailInput, origin SendOrigin, links []ActivityLinkInput, gate ConsentGate) (unsubscribeSurface, error) {
	surface := surfaceFor(in.Context, in.MarketingPurpose, in.ConsentPurpose)
	if surface.disclosureCategory() == commsauthz.CategoryMarketing {
		return surface, nil
	}
	previewer, ok := gate.(SendPreviewer)
	if !ok {
		return surface, nil
	}
	// The question staging will ask, built by the same builder, before the body
	// exists.
	set, err := previewer.Preview(ctx, outboundMessage{in: in, links: links}.authorization(origin))
	if err != nil {
		return unsubscribeSurface{}, err
	}
	if set.AdvertisesToASubject() {
		surface.category = commsauthz.CategoryMarketing
	}
	return surface, nil
}
