// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ChatSummarySource is the source_system of a note summarising chat talk. Its
// raw payload holds what talk it summarises, never the talk itself.
const ChatSummarySource = "dispact"

// chatEpisodeOf answers a chat summary note's raw payload as the talk it
// summarises. Any other row, or a raw of another shape, answers nil.
//
//craft:ignore naked-any raw is the activity's free-form source payload column
func chatEpisodeOf(sourceSystem *string, raw map[string]any) *crmcontracts.ChatEpisode {
	if sourceSystem == nil || *sourceSystem != ChatSummarySource || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var episode crmcontracts.ChatEpisode
	if err := json.Unmarshal(encoded, &episode); err != nil || episode.MessageCount < 1 || episode.Url == "" {
		return nil
	}
	return &episode
}
