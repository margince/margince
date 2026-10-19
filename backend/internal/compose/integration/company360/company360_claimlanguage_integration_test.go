// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAccountClaimTranslatesExplanationAndPreservesQuotedEvidence(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := company360Service(e)
	company := e.SeedCompany(t, "Example Company", &e.Rep1)
	contact := e.SeedContact(t, "Carol", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship(kind,contact_id,company_id,source,captured_by) VALUES('employment',$1,$2,'manual','human:test')`, contact, company)
	subject, quote, direction := "Original subject", "I will send the signed order.", "outbound"
	at := company360Clock.Add(-72 * time.Hour)
	due := company360Clock.Add(-24 * time.Hour)
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &quote, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.RecordConversationClaim(ctx, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_ours", Body: "Send the signed order", Quote: quote, ActivityID: ids.UUID(activity.Id), DueAt: &due, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ lang, why string }{
		{"de", "Seit einem Tag fällig und noch offen."}, {"vi", "Quá hạn một ngày và vẫn đang mở."},
	} {
		if err := settings.Set(ctx, compose.NewSettingsStore(e.Pool), identity.BaseLanguage, tc.lang); err != nil {
			t.Fatal(err)
		}
		page, err := svc.Assemble(ctx, ids.From[ids.CompanyKind](company))
		if err != nil {
			t.Fatal(err)
		}
		if page.Moment == nil || page.Moment.WhyNow != tc.why {
			t.Fatalf("%s: %+v", tc.lang, page.Moment)
		}
		if len(page.Moment.Evidence) != 1 || page.Moment.Evidence[0].Snippet == nil || *page.Moment.Evidence[0].Snippet != quote {
			t.Fatalf("quote changed: %+v", page.Moment.Evidence)
		}
	}
}
