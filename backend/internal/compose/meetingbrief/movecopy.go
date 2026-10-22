// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var moveWords = struct {
	To     langcopy.Phrase
	FromTo langcopy.Phrase
	Offer  langcopy.Phrase
	Buyer  langcopy.Phrase
	Room   langcopy.Phrase
}{
	To:     langcopy.Phrase{En: "Since then the deal moved to %s.", De: "Seitdem ist der Deal in die Phase %s gewechselt.", Vi: "Kể từ đó, giao dịch đã chuyển sang %s."},
	FromTo: langcopy.Phrase{En: "Since then the deal moved from %s to %s.", De: "Seitdem ist der Deal von %s nach %s gewechselt.", Vi: "Kể từ đó, giao dịch đã chuyển từ %s sang %s."},
	Offer:  langcopy.Phrase{En: "Since then offer %s revision %d was %s.", De: "Seitdem hat das Angebot %s (Version %d) den Status %s.", Vi: "Kể từ đó, báo giá %s phiên bản %d có trạng thái %s."},
	Buyer:  langcopy.Phrase{En: "the buyer", De: "der Käufer", Vi: "người mua"},
	Room:   langcopy.Phrase{En: "Since then %s commented in the Deal Room.", De: "Seitdem hat %s im Deal Room kommentiert.", Vi: "Kể từ đó, %s đã bình luận trong Deal Room."},
}

var offerStatusWords = map[string]langcopy.Phrase{
	"draft":      {En: "draft", De: "Entwurf", Vi: "bản nháp"},
	"sent":       {En: "sent", De: "gesendet", Vi: "đã gửi"},
	"accepted":   {En: "accepted", De: "angenommen", Vi: "đã chấp nhận"},
	"rejected":   {En: "rejected", De: "abgelehnt", Vi: "đã từ chối"},
	"expired":    {En: "expired", De: "abgelaufen", Vi: "đã hết hạn"},
	"superseded": {En: "superseded", De: "ersetzt", Vi: "đã được thay thế"},
}

func (said briefCopy) stageMoveLine(from, to string) string {
	if from == "" {
		return fmt.Sprintf(said.Say(moveWords.To), to)
	}
	return fmt.Sprintf(said.Say(moveWords.FromTo), from, to)
}

func (said briefCopy) offerMoveLine(number string, revision int, status string) string {
	if phrase, ok := offerStatusWords[status]; ok {
		status = said.Say(phrase)
	}
	return fmt.Sprintf(said.Say(moveWords.Offer), number, revision, status)
}

func (said briefCopy) roomMoveLine(who string) string {
	if who == "" {
		who = said.Say(moveWords.Buyer)
	}
	return fmt.Sprintf(said.Say(moveWords.Room), who)
}

// English remains the tie-break key so a language change cannot change the selected events.
func (said briefCopy) wordedMove(at time.Time, dealID ids.UUID, render func(briefCopy) string) DealMoveIn {
	return DealMoveIn{At: at, DealID: dealID.String(), Text: render(said), orderText: render(briefCopyFor("en"))}
}
