// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactMomentLanguageDoesNotReopenDismissedWork(t *testing.T) {
	e := Setup(t)
	svc := contactRoomService(e)
	ctx := e.Admin()
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Example Contact", nil))
	subject := "Original promise"
	due := roomFixedNow.Add(48 * time.Hour)
	filed := roomFixedNow.Add(-24 * time.Hour)
	if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: &subject, OccurredAt: &filed, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	baseline, err := svc.Assemble(ctx, contact)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Moment == nil || baseline.Moment.Headline != "You owe them: Original promise" {
		t.Fatalf("baseline: %+v", baseline.Moment)
	}
	dismissal := contracts.DismissContactMomentRequest{ClaimKey: baseline.Moment.ClaimKey, EvidenceFingerprint: baseline.Moment.EvidenceFingerprint}
	if err := svc.DismissMoment(ctx, contact, dismissal); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ language, headline, why string }{
		{"de", "Sie schulden ihnen: Original promise", "Fällig in 2 Tagen."},
		{"vi", "Bạn cần thực hiện cho họ: Original promise", "Đến hạn sau 2 ngày."},
	} {
		if err := settings.Set(ctx, compose.NewSettingsStore(e.Pool), identity.BaseLanguage, tc.language); err != nil {
			t.Fatal(err)
		}
		hidden, err := svc.Assemble(ctx, contact)
		if err != nil {
			t.Fatal(err)
		}
		if hidden.Moment != nil && hidden.Moment.ClaimKey == dismissal.ClaimKey {
			t.Fatal("language reopened dismissed work")
		}
		if err := svc.RestoreMoment(ctx, contact, dismissal); err != nil {
			t.Fatal(err)
		}
		restored, err := svc.Assemble(ctx, contact)
		if err != nil {
			t.Fatal(err)
		}
		if restored.Moment == nil || restored.Moment.Headline != tc.headline || restored.Moment.WhyNow != tc.why {
			t.Fatalf("%s: %+v", tc.language, restored.Moment)
		}
		if restored.Moment.EvidenceFingerprint != dismissal.EvidenceFingerprint || !reflect.DeepEqual(restored.Moment.Evidence, baseline.Moment.Evidence) || !reflect.DeepEqual(restored.Moment.RecommendedAction.Destination, baseline.Moment.RecommendedAction.Destination) {
			t.Fatal("language changed task evidence or action")
		}
		if err := svc.DismissMoment(ctx, contact, dismissal); err != nil {
			t.Fatal(err)
		}
	}
}
