// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// namedByThread narrows several reachable accounts to the one the conversation's
// thread key names as a segment. A merged contact holds an account per chat, and
// the reply belongs to the chat it answers.
func namedByThread(reachable []connector.ChannelIdentity, threadKey string) []connector.ChannelIdentity {
	if len(reachable) < 2 {
		return reachable
	}
	segments := strings.Split(threadKey, ":")
	var named []connector.ChannelIdentity
	for _, identity := range reachable {
		if slices.Contains(segments, identity.ChannelUserID) {
			named = append(named, identity)
		}
	}
	if len(named) == 0 {
		return reachable
	}
	return named
}
