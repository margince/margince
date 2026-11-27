// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

import "fmt"

// RawSourceID is raw_capture's dedupe key for one polled update. It carries the
// bot because update_id counts per bot. A bare update_id would let a replacement
// bot overwrite the payload of an update the outgoing bot delivered. Telegram has
// no history API to fetch that update again.
//
// The key uses the bot id, not the connection id, so a bot reconnected under a new
// connection id still dedupes its own re-deliveries.
func RawSourceID(botID string, updateID int64) string {
	return fmt.Sprintf("%s:%d", botID, updateID)
}
