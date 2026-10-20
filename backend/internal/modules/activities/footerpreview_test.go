// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type previewUnsubscribeLinker struct{ calls, manageCalls int }

func (l *previewUnsubscribeLinker) UnsubscribeToken(context.Context, string, string) (string, bool, error) {
	l.calls++
	return "fixture-token", true, nil
}

func (l *previewUnsubscribeLinker) ManageToken(context.Context, string) (string, bool, error) {
	l.manageCalls++
	return "fixture-manage", true, nil
}

func TestMailFooterPreviewMatchesDeliveryWithoutCreatingLinks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		category    commsauthz.Category
		purpose     string
		unsubscribe bool
	}{
		{"reply", commsauthz.CategoryReplyToInbound, "", false},
		{"marketing", commsauthz.CategoryMarketing, "newsletter", true},
		{"legacy transactional", "", "transactional", false},
		{"legacy marketing", "", "newsletter", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:fixture", Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Create: true}}}})
			linker := &previewUnsubscribeLinker{}
			store := (&Store{}).WithDisclosures(&stubDisclosures{lines: []DisclosureLine{{Kind: "controller_identity", Text: "Example Company"}}}).WithUnsubscribe(linker).WithPublicBaseURL("https://crm.example.test")
			input := SendEmailInput{Context: tc.category, ConsentPurpose: tc.purpose}
			preview, err := store.PreviewMailFooter(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if linker.calls != 0 || linker.manageCalls != 0 {
				t.Fatal("preview created an unsubscribe link")
			}
			if preview.OffersUnsubscribe != tc.unsubscribe || len(preview.Disclosures) != 1 {
				t.Fatalf("footer preview: %+v", preview)
			}
			delivered, err := store.deliverability(ctx, "Message", "Subject", []string{"buyer@example.test"}, surfaceFor(input.Context, input.MarketingPurpose, input.ConsentPurpose))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(delivered.transmitted, preview.Disclosures[0].Text) {
				t.Fatal("delivery omitted previewed disclosure")
			}
			if (linker.calls == 1) != preview.OffersUnsubscribe {
				t.Fatalf("preview unsubscribe=%v, link calls=%d", preview.OffersUnsubscribe, linker.calls)
			}
		})
	}
}

func TestMailFooterPreviewRequiresSendAuthority(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:fixture"})
	if _, err := (&Store{}).PreviewMailFooter(ctx, SendEmailInput{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unauthorized footer preview: %v", err)
	}
}
