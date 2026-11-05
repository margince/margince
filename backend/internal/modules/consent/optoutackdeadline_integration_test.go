// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// owingAcknowledgementWithin is owingAcknowledgement under a pack that bounds
// the receipt, the way the Vietnamese pack bounds it to 24 hours.
func owingAcknowledgementWithin(t *testing.T, e *channelConsentEnv, within time.Duration) *recordingStager {
	t.Helper()
	code := testJurisdiction(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: code, Version: 1, OptOutAcknowledgement: true, OptOutAcknowledgementWithin: within,
	})
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET address_country = upper($2) WHERE id = $1`, e.contact, string(code)); err != nil {
		t.Fatal(err)
	}
	stager := &recordingStager{}
	e.store = e.store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/")
	return stager
}

// TestTheReceiptIsDueWithinThePacksWindowOfTheRefusal pins the deadline to the
// moment the refusal was recorded, on both doors that record one.
func TestTheReceiptIsDueWithinThePacksWindowOfTheRefusal(t *testing.T) {
	refused := time.Date(2026, 11, 5, 9, 30, 0, 0, time.UTC)
	for _, door := range []struct {
		name   string
		refuse func(e *channelConsentEnv) error
	}{
		{"a recorded objection", func(e *channelConsentEnv) error {
			return e.store.Suppress(e.ctx, SuppressInput{
				ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
			})
		}},
		{"an unsubscribe press", func(e *channelConsentEnv) error {
			seedMarketingPurpose(t, e)
			if _, err := e.store.Record(e.ctx, RecordInput{
				ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted", PolicyText: &grantWording,
			}); err != nil {
				return err
			}
			_, err := e.store.PublicStopAllMarketing(e.ctx, e.contact)
			return err
		}},
	} {
		t.Run(door.name, func(t *testing.T) {
			e := setupChannelConsent(t)
			seedSubjectAddress(t, e)
			stager := owingAcknowledgementWithin(t, e, 24*time.Hour)
			e.store.now = func() time.Time { return refused }

			if err := door.refuse(e); err != nil {
				t.Fatalf("recording the refusal: %v", err)
			}
			due := stager.seen.DueAt
			if want := refused.Add(24 * time.Hour); due == nil || !due.Equal(want) {
				t.Fatalf("the receipt is due %v, want %v: 24 hours from the refusal", due, want)
			}
		})
	}
}
