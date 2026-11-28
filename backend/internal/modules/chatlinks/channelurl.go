// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProviderChannelURL builds a provider address from its canonical installation and channel identity.
func ProviderChannelURL(provider, workspace, address, channel string) string {
	switch provider {
	case ProviderSlack:
		return "https://slack.com/app_redirect?" + url.Values{teamKey: {workspace}, channelKey: {channel}}.Encode()
	case ProviderTeams:
		if tenant, _, ok := strings.Cut(workspace, "/"); ok {
			workspace = tenant
		}
		team, native, ok := strings.Cut(channel, "/")
		if !ok {
			return ""
		}
		return "https://teams.microsoft.com/l/channel/" + url.PathEscape(native) + "/channel?" + url.Values{"groupId": {team}, "tenantId": {workspace}}.Encode()
	default:
		if _, err := ids.Parse(channel); err == nil {
			channel = "id:" + channel
		}
		return ChannelURL(address, url.PathEscape(channel))
	}
}
