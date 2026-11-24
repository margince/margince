// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamRequestNoticeKey names one host's request that one colleague join their
// booking link. The writer, the retraction and the mail sender all read it.
func TeamRequestNoticeKey(host, member ids.UserID) string {
	return KindMeetingTeamRequest + ":" + host.String() + ":" + member.String()
}

// hostOfTeamRequestKey is the host a team request notice names.
func hostOfTeamRequestKey(kind, noticeKey string) (ids.UserID, bool) {
	if kind != KindMeetingTeamRequest {
		return ids.UserID{}, false
	}
	parts := strings.Split(noticeKey, ":")
	if len(parts) != 3 || parts[0] != KindMeetingTeamRequest {
		return ids.UserID{}, false
	}
	host, err := ids.Parse(parts[1])
	if err != nil {
		return ids.UserID{}, false
	}
	return ids.From[ids.UserKind](host), true
}
