// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// previewingGate is a send gate whose engine reads each recipient as named.
type previewingGate struct {
	resolved map[string]commsauthz.Category
}

func (previewingGate) RequireGrantedForEmails(context.Context, []string, string) error { return nil }

func (previewingGate) RequireGrantedForRecipients(context.Context, []connector.Recipient, string) error {
	return nil
}

func (g previewingGate) Preview(_ context.Context, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	var set commsauthz.DecisionSet
	for _, r := range req.Recipients {
		set.Decisions = append(set.Decisions, commsauthz.Decision{
			Recipient: r, SubjectKind: "contact", SubjectID: ids.NewV7(), Resolved: g.resolved[r.Email],
		})
	}
	return set, nil
}

// TestACcRecipientAdvertisedToGetsTheAdvertisingDuties: a reply to A with B on
// Cc, where the engine reads B's copy as marketing. One body reaches both, so
// it owes the marketing disclosures and the unsubscribe link. An unsubscribe
// link carries one recipient's token, so the send is refused with the fix:
// one send per recipient. It does not go out to B without them. Sent to B
// alone, the body carries both.
//
// Mutation: return the caller's own surface from surfaceForRecipients and this
// fails.
func TestACcRecipientAdvertisedToGetsTheAdvertisingDuties(t *testing.T) {
	gate := previewingGate{resolved: map[string]commsauthz.Category{
		"a@example.test": commsauthz.CategoryReplyToInbound,
		"b@example.test": commsauthz.CategoryMarketing,
	}}
	disclosures := &stubDisclosures{lines: []DisclosureLine{{Kind: "objection_route", Text: "Reply STOP."}}}
	store := (&Store{}).WithDisclosures(disclosures).WithUnsubscribe(stubUnsubscribeLinker{token: "tok", ok: true})
	store.publicBaseURL = "https://mail.example.test"
	derive := func(in SendEmailInput) (sendDeliverability, error) {
		t.Helper()
		surface, err := surfaceForRecipients(context.Background(), in, SendOrigin{}, nil, gate)
		if err != nil {
			t.Fatal(err)
		}
		if surface.disclosureCategory() != commsauthz.CategoryMarketing || !surface.carries() {
			t.Fatalf("surface %+v: b@ is advertised to, so the body owes the advertising duties", surface)
		}
		return store.deliverability(context.Background(), "As discussed.", "Re: your question", in.Recipients, surface)
	}

	_, err := derive(SendEmailInput{
		Context:    commsauthz.CategoryReplyToInbound,
		Recipients: []string{"a@example.test", "b@example.test"},
		Cc:         []string{"b@example.test"},
	})
	var shared *SharedUnsubscribeTokenError
	if !errors.As(err, &shared) {
		t.Errorf("the reply with an advertised Cc was derived with err %v, want the one-addressee refusal", err)
	}

	got, err := derive(SendEmailInput{Context: commsauthz.CategoryReplyToInbound, Recipients: []string{"b@example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(disclosures.asked, string(commsauthz.CategoryMarketing)) {
		t.Errorf("disclosures were asked for %v, want the marketing ones", disclosures.asked)
	}
	if !strings.Contains(got.transmitted, "Reply STOP.") || got.listUnsubscribe == "" {
		t.Errorf("the body carries no objection route or no unsubscribe link:\n%s", got.transmitted)
	}
}

// TestAReplyToItsOwnThreadStaysCorrespondence is the control: when no
// recipient is advertised to, the reply carries no unsubscribe surface.
func TestAReplyToItsOwnThreadStaysCorrespondence(t *testing.T) {
	gate := previewingGate{resolved: map[string]commsauthz.Category{
		"a@example.test": commsauthz.CategoryReplyToInbound,
	}}
	in := SendEmailInput{Context: commsauthz.CategoryReplyToInbound, Recipients: []string{"a@example.test"}}
	surface, err := surfaceForRecipients(context.Background(), in, SendOrigin{}, nil, gate)
	if err != nil {
		t.Fatal(err)
	}
	if surface.disclosureCategory() == commsauthz.CategoryMarketing || surface.carries() {
		t.Errorf("surface %+v: a reply nobody is advertised to gained the advertising duties", surface)
	}
}
