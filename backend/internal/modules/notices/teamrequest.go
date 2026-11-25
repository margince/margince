// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamRequestNoticeKey names one team link's request that one colleague join
// it. The writer, the retraction and the mail sender all read it.
func TeamRequestNoticeKey(page ids.UUID, member ids.UserID) string {
	return KindMeetingTeamRequest + ":" + page.String() + ":" + member.String()
}

// pageOfTeamRequestKey is the team link a team request notice names.
func pageOfTeamRequestKey(kind, noticeKey string) (ids.UUID, bool) {
	if kind != KindMeetingTeamRequest {
		return ids.Nil, false
	}
	parts := strings.Split(noticeKey, ":")
	if len(parts) != 3 || parts[0] != KindMeetingTeamRequest {
		return ids.Nil, false
	}
	page, err := ids.Parse(parts[1])
	if err != nil {
		return ids.Nil, false
	}
	return page, true
}
