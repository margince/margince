// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import "context"

// SourceReader resolves source links under the participant's current grant.
type SourceReader interface {
	Source(context.Context, string, string) (string, error)
}

// ContainsLiveRoot requires live root evidence before a source can be opened.
func ContainsLiveRoot(page Page, root string) bool {
	for _, message := range page.Messages {
		if message.ID == root && !message.Deleted {
			return true
		}
	}
	return false
}
