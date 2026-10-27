// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// StoredOriginalSQL is true when the raw_capture row aliased raw is the
// original the activity aliased activity was read from.
//
// activity.raw_capture_id is the link the activity writer stamps. A Telegram
// original needs it: the poll keys raw_capture on the provider's redelivery
// key, and the activity on the chat and message. Rows written before the
// column existed fall back to the key join, which was exact for mail.
func StoredOriginalSQL(activity, raw string) string {
	return fmt.Sprintf(`(%[2]s.id = %[1]s.raw_capture_id
	 OR (%[1]s.raw_capture_id IS NULL
	     AND %[2]s.source_system = %[1]s.source_system AND %[2]s.source_id = %[1]s.source_id))`, activity, raw)
}
