// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestSharedSendDecodersPreserveContextValidation(t *testing.T) {
	for _, category := range commsauthz.Categories() {
		t.Run(string(category), func(t *testing.T) {
			email, emailErr := DecodeCompanyEmailRequest(crmcontracts.SendCompanyEmailRequest{Body: "Review the attached scope", CommunicationContext: new(crmcontracts.SendCompanyEmailRequestCommunicationContext(category))})
			channel, channelErr := DecodeSendMessageRequest(crmcontracts.SendMessageRequest{Body: "Review the attached scope", CommunicationContext: new(crmcontracts.SendMessageRequestCommunicationContext(category))})
			if category.ServesTheSubject() {
				if emailErr == nil || channelErr == nil {
					t.Fatalf("reserved category accepted: email=%v channel=%v", emailErr, channelErr)
				}
				return
			}
			if emailErr != nil || channelErr != nil {
				t.Fatalf("ordinary category refused: email=%v channel=%v", emailErr, channelErr)
			}
			if email.Context != category || channel.Context != category {
				t.Fatalf("claim changed: email=%v channel=%v", email.Context, channel.Context)
			}
		})
	}
}

func TestSharedSendDecodersRefuseUnknownClaimsAndOversizedReasons(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		category, reason := "unknown_category", "A first message"
		if oversized {
			category, reason = string(commsauthz.CategoryActiveDealFollowup), strings.Repeat("x", maxOperatorReasonRunes+1)
		}
		_, emailErr := DecodeCompanyEmailRequest(crmcontracts.SendCompanyEmailRequest{CommunicationContext: new(crmcontracts.SendCompanyEmailRequestCommunicationContext(category)), OperatorReason: &reason})
		_, channelErr := DecodeSendMessageRequest(crmcontracts.SendMessageRequest{CommunicationContext: new(crmcontracts.SendMessageRequestCommunicationContext(category)), OperatorReason: &reason})
		if emailErr == nil || channelErr == nil {
			t.Fatalf("oversized=%t: email=%v channel=%v", oversized, emailErr, channelErr)
		}
	}
}
