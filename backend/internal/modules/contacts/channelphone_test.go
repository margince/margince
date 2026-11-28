// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestOnlyAnUnscopedWhatsAppPhoneReachesThePhoneFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity connector.ChannelIdentity
		want     string
	}{
		{"phone", connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "491701234567"}, "+491701234567"},
		{"business user", connector.ChannelIdentity{Provider: "whatsapp", AccountScope: "business", ChannelUserID: "491701234567"}, ""},
		{"telegram user", connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "491701234567"}, ""},
		{"zalo user", connector.ChannelIdentity{Provider: "zalo_oa", ChannelUserID: "491701234567"}, ""},
		{"opaque user", connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "BR.1234"}, ""},
		{"invalid number", connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "0123456789"}, ""},
		{"formatted identity", connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "49 1701234567"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := whatsAppPhone(test.identity); got != test.want {
				t.Errorf("phone = %q, want %q", got, test.want)
			}
		})
	}
}
