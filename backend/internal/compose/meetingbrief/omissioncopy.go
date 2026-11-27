// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/datenotation"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// briefCopy words the brief in its language and writes its dates in the
// installation's notation.
type briefCopy struct {
	langcopy.Spoken
	dates datenotation.Notation
}

func briefCopyFor(language string) briefCopy {
	spoken := langcopy.For(language)
	return briefCopy{Spoken: spoken, dates: datenotation.Notation{Lang: spoken.Lang()}}
}

// words is the brief's copy with the dates this read resolved.
func (in Input) words() briefCopy {
	said := briefCopyFor(in.Language)
	if in.Dates.Lang != "" {
		said.dates = in.Dates
	}
	return said
}

var omissionWords = struct{ Room, OneHistory, History langcopy.Phrase }{
	Room:       langcopy.Phrase{En: "You do not have access to Deal Rooms, so what the buyer did in this deal's room is not in this brief.", De: "Du hast keinen Zugriff auf Deal Rooms. Die Aktivitäten der Käuferseite im Deal Room sind daher nicht in dieser Vorbereitung enthalten.", Vi: "Bạn không có quyền truy cập Deal Room, nên bản chuẩn bị này không bao gồm hoạt động của người mua trong Deal Room của giao dịch này."},
	OneHistory: langcopy.Phrase{En: "1 conversation with this company is not yours to read, so the relationship history is built from the rest.", De: "Du darfst ein Gespräch zu diesem Unternehmen nicht lesen. Der Beziehungsverlauf basiert daher auf den übrigen Gesprächen.", Vi: "Bạn không có quyền đọc một cuộc trao đổi của khách hàng này, nên diễn biến quan hệ được tổng hợp từ những cuộc trao đổi còn lại."},
	History:    langcopy.Phrase{En: "%d conversations with this company are not yours to read, so the relationship history is built from the rest.", De: "Du darfst %d Gespräche zu diesem Unternehmen nicht lesen. Der Beziehungsverlauf basiert daher auf den übrigen Gesprächen.", Vi: "Bạn không có quyền đọc %d cuộc trao đổi của khách hàng này, nên diễn biến quan hệ được tổng hợp từ những cuộc trao đổi còn lại."},
}

func (said briefCopy) withheldHistory(count int) string {
	if count == 1 {
		return said.Say(omissionWords.OneHistory)
	}
	return fmt.Sprintf(said.Say(omissionWords.History), count)
}
