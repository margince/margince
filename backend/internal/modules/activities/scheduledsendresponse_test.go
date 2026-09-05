// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"reflect"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// What a scheduled row says about itself on the wire is what a surface asks the
// engine with. Every input the frozen payload holds that the preview door
// accepts has to survive the render, and nothing the wire cannot say may leak
// through it.

func scheduledRow() ScheduledSend {
	return ScheduledSend{
		ID:          ids.NewV7(),
		Status:      ScheduledStatusScheduled,
		Subject:     "Your quote",
		Recipients:  []string{"buyer@example.test"},
		ScheduledBy: ids.NewV7(),
		Version:     1,
	}
}

func TestTheWireCarriesEveryInputThePreviewAsksWith(t *testing.T) {
	t.Parallel()
	deal := ids.NewV7()
	org := ids.NewV7()
	row := scheduledRow()
	row.Links = []ActivityLinkInput{{EntityType: "organization", EntityID: org}}
	row.Context = commsauthz.CategoryMarketing
	row.MarketingPurpose = "newsletter"
	row.ConsentPurpose = "marketing_email"
	row.Evidence = commsauthz.Evidence{DealID: deal}

	out := scheduledSendResponse(row)

	wantLinks := []crmcontracts.ActivityLinkInput{{
		EntityType: crmcontracts.ActivityLinkInputEntityType("organization"),
		EntityId:   openapi_types.UUID(org),
	}}
	if out.Links == nil || !reflect.DeepEqual(*out.Links, wantLinks) {
		t.Errorf("links on the wire = %v, want %v", out.Links, wantLinks)
	}
	if out.CommunicationContext == nil || *out.CommunicationContext != crmcontracts.CommunicationContext("marketing") {
		t.Errorf("claim on the wire = %v, want marketing", out.CommunicationContext)
	}
	if out.MarketingPurpose == nil || *out.MarketingPurpose != "newsletter" {
		t.Errorf("marketing purpose on the wire = %v, want newsletter", out.MarketingPurpose)
	}
	if out.ConsentPurpose == nil || *out.ConsentPurpose != "marketing_email" {
		t.Errorf("consent purpose on the wire = %v, want marketing_email", out.ConsentPurpose)
	}
	if out.Evidence == nil || out.Evidence.DealId == nil || ids.UUID(*out.Evidence.DealId) != deal {
		t.Errorf("evidence on the wire = %+v, want the deal named", out.Evidence)
	}
	if out.Evidence != nil && out.Evidence.InvoiceId != nil {
		t.Errorf("evidence names an invoice nobody named: %v", out.Evidence.InvoiceId)
	}
}

// A message that claimed nothing says nothing, rather than sending empty
// strings and a block of six nulls a client would have to read as "none".
func TestAMessageThatClaimedNothingSaysNothing(t *testing.T) {
	t.Parallel()
	out := scheduledSendResponse(scheduledRow())
	if out.Links != nil || out.CommunicationContext != nil || out.MarketingPurpose != nil ||
		out.ConsentPurpose != nil || out.Evidence != nil {
		t.Errorf("a row with no claim rendered one: %+v", out)
	}
}

// The store type names the five controller-only categories; the wire's enum
// does not. A row carrying one — a legacy row, or a writer the fire's own guard
// exists to catch — must not reach a client as a value outside its contract.
func TestAControllerOnlyCategoryNeverReachesTheWire(t *testing.T) {
	t.Parallel()
	row := scheduledRow()
	row.Context = commsauthz.CategorySecurityNotice
	if out := scheduledSendResponse(row); out.CommunicationContext != nil {
		t.Errorf("a controller-only category reached the wire as %q", *out.CommunicationContext)
	}
	row.Context = commsauthz.Category("a_category_this_build_does_not_know")
	if out := scheduledSendResponse(row); out.CommunicationContext != nil {
		t.Errorf("an unknown category reached the wire as %q", *out.CommunicationContext)
	}
}

// The records an account send freezes come back exactly as frozen, and a reply
// — which freezes none — reads as none rather than as an error. One reader for
// the list and the fire, so this holds both.
func TestFrozenRecordLinksRoundTrip(t *testing.T) {
	t.Parallel()
	links := []ActivityLinkInput{
		{EntityType: "organization", EntityID: ids.NewV7()},
		{EntityType: "deal", EntityID: ids.NewV7()},
	}
	frozen, err := marshalOriginLinks(FromAccount(links))
	if err != nil {
		t.Fatalf("freezing: %v", err)
	}
	thawed, err := thawOriginLinks(frozen)
	if err != nil {
		t.Fatalf("thawing: %v", err)
	}
	if !reflect.DeepEqual(thawed, links) {
		t.Errorf("records came back changed:\n frozen: %+v\n thawed: %+v", links, thawed)
	}

	replyFrozen, err := marshalOriginLinks(FromActivity(ids.ActivityID{UUID: ids.NewV7()}))
	if err != nil {
		t.Fatalf("freezing a reply: %v", err)
	}
	replyThawed, err := thawOriginLinks(replyFrozen)
	if err != nil {
		t.Fatalf("a reply's NULL records read as an error: %v", err)
	}
	if replyThawed != nil {
		t.Errorf("a reply thawed records it never froze: %+v", replyThawed)
	}
}
