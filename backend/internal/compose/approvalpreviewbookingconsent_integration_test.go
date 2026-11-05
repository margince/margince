// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBookingConsentPreviewPreservesRecordedDecisions(t *testing.T) {
	for _, current := range []string{"", "granted", "withdrawn"} {
		t.Run("current="+current, func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["consent_config"] = principal.ObjectGrant{Read: true, Create: true}
			ctx := e.As(e.AdminUser, nil, perms)
			contact := e.SeedContact(t, "Dana Buyer", &e.AdminUser)
			store := consent.NewStore(e.DB())
			purpose, err := store.CreatePurpose(ctx, "transactional", "Meeting correspondence", false)
			if err != nil {
				t.Fatal(err)
			}
			request := activities.BookingConsent{PurposeID: purpose.ID.UUID, PolicyVersion: "meeting-v1", Wording: "Contact me about this meeting."}
			if current != "" {
				initial := bookingConsentRecord(contact, request)
				initial.NewState = current
				if _, err := store.Record(ctx, initial); err != nil {
					t.Fatal(err)
				}
			}
			before := e.WsCount(t, `SELECT count(*) FROM consent_event`)
			payload := fmt.Sprintf(`{"operation":"bookMeeting","body":{"start":"2027-01-12T09:00:00Z","end":"2027-01-12T10:00:00Z","links":[{"entity_type":"contact","entity_id":%q}],"consent":{"purpose_id":%q,"policy_version":%q,"wording":%q}}}`, contact, purpose.ID, request.PolicyVersion, request.Wording)
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "book_meeting", TargetType: "contact", TargetID: contact, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]string{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value.Text
			}
			want := current
			if want == "" {
				want = "granted"
			}
			if details["consent_result"] != want || details["consent_wording"] != request.Wording || details["consent_purpose"] != purpose.Label {
				t.Fatalf("consent details: %+v", details)
			}
			if after := e.WsCount(t, `SELECT count(*) FROM consent_event`); after != before {
				t.Fatalf("preview recorded consent: %d -> %d", before, after)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM confirm_token`); count != 0 {
				t.Fatalf("preview minted %d tokens", count)
			}
			if _, err := (bookingConsentAdapter{store: store}).CaptureBookingConsent(ctx, contact, request); err != nil {
				t.Fatal(err)
			}
			expectedProofs := before
			if current == "" {
				expectedProofs++
			}
			if after := e.WsCount(t, `SELECT count(*) FROM consent_event`); after != expectedProofs {
				t.Fatalf("writer proofs=%d, want %d", after, expectedProofs)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2 AND state = $3`, ids.From[ids.ContactKind](contact), purpose.ID, want); count != 1 {
				t.Fatalf("writer disagrees with preview state %q", want)
			}
		})
	}
}
