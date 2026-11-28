// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import "encoding/json"

// AppManifest keeps installation callbacks, scopes and event subscriptions derived from their owners.
func AppManifest(personalCallback, channelCallback, eventsURL string) (string, error) {
	var manifest struct {
		Display  map[string]string `json:"display_information"`
		Features struct {
			Bot struct {
				Name   string `json:"display_name"`
				Online bool   `json:"always_online"`
			} `json:"bot_user"`
		} `json:"features"`
		OAuth struct {
			Redirects []string `json:"redirect_urls"`
			Scopes    struct {
				Bot  []string `json:"bot"`
				User []string `json:"user"`
			} `json:"scopes"`
		} `json:"oauth_config"`
		Settings struct {
			Events struct {
				URL    string   `json:"request_url"`
				Events []string `json:"bot_events"`
			} `json:"event_subscriptions"`
			Rotation bool `json:"token_rotation_enabled"`
		} `json:"settings"`
	}
	manifest.Display = map[string]string{"name": "Margince", "description": "Record messaging and reviewed channel updates"}
	manifest.Features.Bot.Name = "Margince"
	manifest.OAuth.Redirects = []string{personalCallback, channelCallback}
	manifest.OAuth.Scopes.Bot, manifest.OAuth.Scopes.User = BotScopes, UserScopes
	manifest.Settings.Events.URL, manifest.Settings.Events.Events = eventsURL, []string{"message.groups", "message.channels"}
	manifest.Settings.Rotation = true
	raw, err := json.MarshalIndent(manifest, "", "  ")
	return string(raw), err
}
