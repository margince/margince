// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type briefCopy struct{ langcopy.Spoken }

func briefCopyFor(language string) briefCopy { return briefCopy{langcopy.For(language)} }

var omissionWords = struct{ Room, OneHistory, History langcopy.Phrase }{
	Room:       langcopy.Phrase{En: "You do not have access to Deal Rooms, so what the buyer did in this deal's room is not in this brief.", De: "Du hast keinen Zugriff auf Deal Rooms. Die Aktivitäten des Käufers im Deal Room sind daher nicht in dieser Vorbereitung enthalten.", Vi: "Bạn không có quyền truy cập Deal Room, nên bản chuẩn bị này không bao gồm hoạt động của người mua trong Deal Room của giao dịch này."},
	OneHistory: langcopy.Phrase{En: "1 conversation in this account is not yours to read, so the account arc is built from the rest.", De: "Du darfst ein Gespräch zu diesem Unternehmen nicht lesen. Der Beziehungsverlauf basiert daher auf den übrigen Gesprächen.", Vi: "Bạn không có quyền đọc một cuộc trao đổi của khách hàng này, nên diễn biến quan hệ được tổng hợp từ những cuộc trao đổi còn lại."},
	History:    langcopy.Phrase{En: "%d conversations in this account are not yours to read, so the account arc is built from the rest.", De: "Du darfst %d Gespräche zu diesem Unternehmen nicht lesen. Der Beziehungsverlauf basiert daher auf den übrigen Gesprächen.", Vi: "Bạn không có quyền đọc %d cuộc trao đổi của khách hàng này, nên diễn biến quan hệ được tổng hợp từ những cuộc trao đổi còn lại."},
}

func (said briefCopy) withheldHistory(count int) string {
	if count == 1 {
		return said.Say(omissionWords.OneHistory)
	}
	return fmt.Sprintf(said.Say(omissionWords.History), count)
}
