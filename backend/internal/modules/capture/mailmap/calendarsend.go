// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
)

// calendarSend names the one send a calendar notice is a copy of, or "".
//
// Google Calendar mails each guest their own copy of an invitation, update or
// cancellation, and files every copy in the organizer's Sent. Each copy has its
// own Message-ID, To line and reply link, and the same calendar part. So the
// part names the send, and a later send that changes any of it names another.
func calendarSend(notice bool, method, uid string, event *calendarevent.Event) string {
	if !notice || method == "" || uid == "" || event == nil {
		return ""
	}
	stated, err := json.Marshal(event)
	if err != nil {
		// Named no send, the copy is captured as its own activity.
		return ""
	}
	sum := sha256.Sum256(append([]byte(uid+"\x00"+method+"\x00"), stated...))
	return hex.EncodeToString(sum[:])
}
