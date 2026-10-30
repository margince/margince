// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

// ownershipTransferWords are the sentences a transfer's card and its notices
// carry, in the installation's base language. The record and colleague names
// are quoted as they are stored.
var ownershipTransferWords = struct {
	// AskedContact and AskedCompany: the colleague asking, then the count of
	// open deals and of open tasks that move with the record.
	AskedContact, AskedCompany langcopy.Phrase
	// Accepted: the colleague who handed it over, then the record.
	Accepted langcopy.Phrase
	// AcceptedMoved: the deals, then the tasks that moved with it.
	AcceptedMoved langcopy.Phrase
	// KeptDeals: the colleague the deals stayed with, then how many.
	KeptDeals langcopy.Phrase
	// KeptTasks: the colleague the tasks stayed with, then how many.
	KeptTasks langcopy.Phrase
	// Declined, Expired and Withdrawn: the owner, then the record.
	Declined, DeclinedBody   langcopy.Phrase
	Expired, ExpiredBody     langcopy.Phrase
	Withdrawn, WithdrawnBody langcopy.Phrase
}{
	AskedContact: langcopy.Phrase{
		En: "%s asks you to hand over this contact. Open deals that move with it: %d. Open tasks: %d.",
		De: "%s bittet dich, diesen Kontakt zu übergeben. Offene Deals, die mitgehen: %d. Offene Aufgaben: %d.",
		Vi: "%s đề nghị bạn chuyển giao liên hệ này. Số giao dịch đang mở đi kèm: %d. Số việc đang mở: %d.",
	},
	AskedCompany: langcopy.Phrase{
		En: "%s asks you to hand over this company. Open deals that move with it: %d. Open tasks: %d.",
		De: "%s bittet dich, dieses Unternehmen zu übergeben. Offene Deals, die mitgehen: %d. Offene Aufgaben: %d.",
		Vi: "%s đề nghị bạn chuyển giao công ty này. Số giao dịch đang mở đi kèm: %d. Số việc đang mở: %d.",
	},
	Accepted: langcopy.Phrase{
		En: "%s handed %s over to you",
		De: "%s hat dir %s übergeben",
		Vi: "%s đã chuyển giao %s cho bạn",
	},
	AcceptedMoved: langcopy.Phrase{
		En: "It is yours now. Open deals that moved with it: %d. Open tasks: %d.",
		De: "Es gehört jetzt dir. Offene Deals, die mitgegangen sind: %d. Offene Aufgaben: %d.",
		Vi: "Giờ nó thuộc về bạn. Số giao dịch đang mở đã chuyển theo: %d. Số việc đang mở: %d.",
	},
	KeptDeals: langcopy.Phrase{
		En: "Open deals that stayed with %s, because they belong to other records too or could not be moved: %d.",
		De: "Offene Deals, die bei %s geblieben sind, weil sie auch zu anderen Datensätzen gehören oder sich nicht übertragen ließen: %d.",
		Vi: "Số giao dịch đang mở vẫn ở lại với %s vì còn thuộc bản ghi khác hoặc không thể chuyển: %d.",
	},
	KeptTasks: langcopy.Phrase{
		En: "Open tasks that stayed with %s, because they belong to other records too or could not be moved: %d.",
		De: "Offene Aufgaben, die bei %s geblieben sind, weil sie auch zu anderen Datensätzen gehören oder sich nicht übertragen ließen: %d.",
		Vi: "Số việc đang mở vẫn ở lại với %s vì còn thuộc bản ghi khác hoặc không thể chuyển: %d.",
	},
	Declined: langcopy.Phrase{
		En: "%s keeps %s",
		De: "%s behält %s",
		Vi: "%s giữ lại %s",
	},
	DeclinedBody: langcopy.Phrase{
		En: "Your request to take it over was Declined. You can ask again.",
		De: "Deine Bitte um Übergabe wurde abgelehnt. Du kannst erneut fragen.",
		Vi: "Đề nghị nhận lại của bạn đã bị từ chối. Bạn có thể hỏi lại.",
	},
	Expired: langcopy.Phrase{
		En: "%s did not answer about %s",
		De: "%s hat zu %s nicht geantwortet",
		Vi: "%s chưa trả lời về %s",
	},
	ExpiredBody: langcopy.Phrase{
		En: "Your request ran out after 7 days with no answer. You can ask again.",
		De: "Deine Bitte ist nach 7 Tagen ohne Antwort abgelaufen. Du kannst erneut fragen.",
		Vi: "Đề nghị của bạn đã hết hạn sau 7 ngày không có trả lời. Bạn có thể hỏi lại.",
	},
	Withdrawn: langcopy.Phrase{
		En: "%s no longer owns %s",
		De: "%s ist nicht mehr Inhaber von %s",
		Vi: "%s không còn sở hữu %s",
	},
	WithdrawnBody: langcopy.Phrase{
		En: "The record changed hands, so your request was closed. Ask its new owner if you still need it.",
		De: "Der Datensatz hat den Inhaber gewechselt, deshalb wurde deine Bitte geschlossen. Frag den neuen Inhaber, wenn du ihn noch brauchst.",
		Vi: "Bản ghi đã đổi chủ nên đề nghị của bạn đã được đóng. Hãy hỏi chủ mới nếu bạn vẫn cần.",
	},
}
