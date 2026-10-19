// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var coachingWords = struct {
	Listen      langcopy.Phrase
	Intervene   langcopy.Phrase
	Outcome     langcopy.Phrase
	Purpose     langcopy.Phrase
	History     langcopy.Phrase
	Asks        langcopy.Phrase
	Assume      langcopy.Phrase
	Silence     langcopy.Phrase
	Coverage    langcopy.Phrase
	Questions   langcopy.Phrase
	NoQuestions langcopy.Phrase
}{
	Listen:      langcopy.Phrase{En: "A quantified consequence in their words, and who owns it today.", De: "Eine bezifferte Folge in ihren Worten und die Person, die heute dafür verantwortlich ist.", Vi: "Một hệ quả được định lượng bằng lời của họ và người đang chịu trách nhiệm."},
	Intervene:   langcopy.Phrase{En: "A date, a price or a resource is promised that nobody on our side has agreed.", De: "Ein Termin, ein Preis oder eine Ressource wird zugesagt, dem auf unserer Seite niemand zugestimmt hat.", Vi: "Một ngày, mức giá hoặc nguồn lực được hứa mà phía chúng ta chưa ai đồng ý."},
	Outcome:     langcopy.Phrase{En: "Whether the rep leaves with the outcome the plan names, or settles for a pleasant conversation.", De: "Ob die Vertriebsperson das im Plan benannte Ergebnis erreicht oder sich mit einem angenehmen Gespräch zufriedengibt.", Vi: "Liệu nhân viên bán hàng đạt được kết quả nêu trong kế hoạch hay chỉ hài lòng với một cuộc trò chuyện dễ chịu."},
	Purpose:     langcopy.Phrase{En: "Whether the rep establishes what this meeting is for before spending it.", De: "Ob die Vertriebsperson den Zweck des Meetings klärt, bevor sie die Zeit nutzt.", Vi: "Liệu nhân viên bán hàng làm rõ mục đích cuộc họp trước khi sử dụng thời gian đó."},
	History:     langcopy.Phrase{En: "Defending the history instead of owning it and naming a date.", De: "Die Vergangenheit zu verteidigen, statt Verantwortung zu übernehmen und einen Termin zu nennen.", Vi: "Biện hộ cho chuyện đã qua thay vì nhận trách nhiệm và nêu ngày cụ thể."},
	Asks:        langcopy.Phrase{En: "Answering the asks one by one and never getting to a question of their own.", De: "Eine Frage nach der anderen zu beantworten, ohne selbst eine zu stellen.", Vi: "Trả lời từng câu hỏi mà không bao giờ đặt được câu hỏi của mình."},
	Assume:      langcopy.Phrase{En: "Assuming what the meeting is for, and finding out at the end that it was not.", De: "Den Zweck des Meetings vorauszusetzen und erst am Ende zu merken, dass es um etwas anderes ging.", Vi: "Tự cho rằng biết mục đích cuộc họp rồi đến cuối mới nhận ra không phải như vậy."},
	Silence:     langcopy.Phrase{En: "Filling the silence with product rather than asking what is actually going on.", De: "Die Stille mit Produktinformationen zu füllen, statt nach der tatsächlichen Situation zu fragen.", Vi: "Lấp khoảng lặng bằng thông tin sản phẩm thay vì hỏi điều gì thực sự đang diễn ra."},
	Coverage:    langcopy.Phrase{En: "Proving coverage too early, before the problem is agreed.", De: "Die Abdeckung zu früh nachweisen zu wollen, bevor Einigkeit über das Problem besteht.", Vi: "Chứng minh khả năng đáp ứng quá sớm, trước khi thống nhất về vấn đề."},
	Questions:   langcopy.Phrase{En: "The rep answering before asking. Count the questions they get to.", De: "Die Vertriebsperson antwortet, bevor sie fragt. Zähle, wie viele Fragen sie stellt.", Vi: "Nhân viên bán hàng trả lời trước khi hỏi. Đếm số câu hỏi họ đặt được."},
	NoQuestions: langcopy.Phrase{En: "A conversation with no question in it.", De: "Ein Gespräch ohne eine einzige Frage.", Vi: "Một cuộc trò chuyện không có câu hỏi nào."},
}
