// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var objectiveWords = struct {
	CaveatRelationship langcopy.Phrase
	CaveatDiscovery    langcopy.Phrase
	CaveatCommercial   langcopy.Phrase
	CaveatDelivery     langcopy.Phrase
	CaveatUnknown      langcopy.Phrase
	CaveatDemo         langcopy.Phrase
	Priority           langcopy.Phrase
	Overdue            langcopy.Phrase
	Answer             langcopy.Phrase
	Unknown            langcopy.Phrase
	Discovery          langcopy.Phrase
	Relationship       langcopy.Phrase
	Delivery           langcopy.Phrase
	Commercial         langcopy.Phrase
	Decision           langcopy.Phrase
	Renewal            langcopy.Phrase
	Demo               langcopy.Phrase
	Next               langcopy.Phrase
	Prior              langcopy.Phrase
	Opening            langcopy.Phrase
	Changed            langcopy.Phrase
	Acknowledge        langcopy.Phrase
	ShowAnswer         langcopy.Phrase
	AvoidArgument      langcopy.Phrase
	OwnDelay           langcopy.Phrase
	ShowReady          langcopy.Phrase
	AvoidDate          langcopy.Phrase
}{
	CaveatRelationship: langcopy.Phrase{En: "Do not force a commercial agenda onto an informal meeting. Ask permission first, then earn the next step.", De: "Dränge einem informellen Treffen keine Verkaufsagenda auf. Frage zuerst, ob das Thema passt, und gewinne dann den nächsten Schritt.", Vi: "Đừng áp đặt nội dung bán hàng lên một cuộc gặp thân mật. Xin phép trước, rồi tạo cơ sở cho bước tiếp theo."},
	CaveatDiscovery:    langcopy.Phrase{En: "Do not pitch. The meeting is worth more as questions than as answers.", De: "Halte keine Verkaufspräsentation. Fragen bringen in diesem Meeting mehr als Antworten.", Vi: "Đừng chào bán. Cuộc họp này có giá trị hơn khi bạn đặt câu hỏi thay vì đưa ra câu trả lời."},
	CaveatCommercial:   langcopy.Phrase{En: "Do not concede on price before the value is agreed.", De: "Mache keine Preiszugeständnisse, bevor der Wert geklärt ist.", Vi: "Đừng nhượng bộ về giá trước khi thống nhất được giá trị."},
	CaveatDelivery:     langcopy.Phrase{En: "Do not reopen scope that is already agreed; name what changed and what it costs.", De: "Stelle den vereinbarten Umfang nicht erneut zur Diskussion. Benenne die Änderungen und ihre Kosten.", Vi: "Đừng mở lại phạm vi đã thống nhất; hãy nêu rõ điều gì thay đổi và chi phí của thay đổi đó."},
	CaveatUnknown:      langcopy.Phrase{En: "Do not assume what this meeting is for. Ask, then plan the rest of the hour.", De: "Setze den Zweck des Meetings nicht voraus. Frage danach und plane dann die verbleibende Zeit.", Vi: "Đừng tự cho rằng bạn biết mục đích cuộc họp. Hãy hỏi, rồi lên kế hoạch cho thời gian còn lại."},
	CaveatDemo:         langcopy.Phrase{En: "Do not tour the product. Show the two things they asked about and stop.", De: "Führe nicht durch das ganze Produkt. Zeige die zwei angefragten Punkte und höre dann auf.", Vi: "Đừng giới thiệu toàn bộ sản phẩm. Chỉ trình bày hai điều họ đã hỏi rồi dừng lại."},
	Priority:           langcopy.Phrase{En: "Confirm with %s whether this is still a priority, and leave with one dated next step on it: %s", De: "Kläre mit %s, ob dies weiterhin Priorität hat, und vereinbare dazu einen nächsten Schritt mit Termin: %s", Vi: "Xác nhận với %s xem đây có còn là ưu tiên không và thống nhất một bước tiếp theo có ngày cụ thể: %s"},
	Overdue:            langcopy.Phrase{En: "Close out what we owe %s, overdue since %s, and agree the next step: %s", De: "Erledige, was wir %s schulden und seit %s überfällig ist, und vereinbare den nächsten Schritt: %s", Vi: "Hoàn tất điều chúng ta còn nợ %s, đã quá hạn từ %s, và thống nhất bước tiếp theo: %s"},
	Answer:             langcopy.Phrase{En: "Leave with %s's answer on: %s", De: "Erhalte die Antwort von %s zu: %s", Vi: "Kết thúc cuộc họp với câu trả lời của %s về: %s"},
	Unknown:            langcopy.Phrase{En: "Establish what this meeting is for before planning the rest of the hour.", De: "Kläre den Zweck dieses Meetings, bevor du die verbleibende Zeit planst.", Vi: "Làm rõ mục đích cuộc họp trước khi lên kế hoạch cho thời gian còn lại."},
	Discovery:          langcopy.Phrase{En: "Learn what prompted this conversation and who else decides, and earn a second meeting.", De: "Finde heraus, was dieses Gespräch ausgelöst hat und wer mitentscheidet, und gewinne ein zweites Treffen.", Vi: "Tìm hiểu điều gì dẫn đến cuộc trao đổi và ai khác tham gia quyết định, rồi tạo cơ sở cho cuộc họp thứ hai."},
	Relationship:       langcopy.Phrase{En: "Keep the relationship warm and find out what has changed since you last spoke.", De: "Pflege die Beziehung und finde heraus, was sich seit dem letzten Gespräch verändert hat.", Vi: "Duy trì quan hệ và tìm hiểu điều gì đã thay đổi từ lần trao đổi trước."},
	Delivery:           langcopy.Phrase{En: "Agree what is done, what is next, and who owns it.", De: "Vereinbare, was erledigt ist, was als Nächstes ansteht und wer dafür verantwortlich ist.", Vi: "Thống nhất việc gì đã xong, việc gì tiếp theo và ai chịu trách nhiệm."},
	Commercial:         langcopy.Phrase{En: "Agree the shape of the commercial terms, or name what is blocking them.", De: "Einige dich mit ihnen auf die kommerziellen Eckpunkte oder benenne, was sie noch verhindert.", Vi: "Thống nhất các điều khoản thương mại chính hoặc nêu rõ điều gì đang cản trở."},
	Decision:           langcopy.Phrase{En: "Get the decision, or the date and the name of whoever makes it.", De: "Erhalte die Entscheidung oder den Termin und den Namen der entscheidenden Person.", Vi: "Lấy được quyết định hoặc ngày ra quyết định và tên người quyết định."},
	Renewal:            langcopy.Phrase{En: "Understand what went wrong, and agree one thing that changes before the renewal.", De: "Verstehe, was schiefgelaufen ist, und vereinbare eine Änderung vor der Verlängerung.", Vi: "Hiểu điều gì đã sai và thống nhất một thay đổi trước khi gia hạn."},
	Demo:               langcopy.Phrase{En: "Show the two things they said they needed, and agree what happens after.", De: "Zeige die zwei Dinge, die sie benötigen, und vereinbare, was danach geschieht.", Vi: "Trình bày hai điều họ nói là cần và thống nhất việc gì sẽ diễn ra sau đó."},
	Next:               langcopy.Phrase{En: "Agree a dated next step before the meeting ends.", De: "Vereinbare vor Ende des Meetings einen nächsten Schritt mit Termin.", Vi: "Thống nhất một bước tiếp theo có ngày cụ thể trước khi kết thúc cuộc họp."},
	Prior:              langcopy.Phrase{En: "Open on what was agreed at %q on %s, and ask what has moved since.", De: "Beginne mit den Vereinbarungen aus %q vom %s und frage, was sich seitdem getan hat.", Vi: "Mở đầu bằng những gì đã thống nhất tại %q vào %s và hỏi điều gì đã tiến triển từ đó."},
	Opening:            langcopy.Phrase{En: "Open by asking what prompted the meeting and what would make the hour worth their time.", De: "Frage zu Beginn nach dem Anlass des Meetings und danach, was die Zeit für sie wertvoll machen würde.", Vi: "Mở đầu bằng việc hỏi lý do cuộc họp và điều gì sẽ khiến khoảng thời gian này có giá trị với họ."},
	Changed:            langcopy.Phrase{En: "Open on what has changed since you last spoke, before proposing anything.", De: "Beginne mit den Veränderungen seit dem letzten Gespräch, bevor du etwas vorschlägst.", Vi: "Mở đầu bằng những thay đổi từ lần trao đổi trước, trước khi đề xuất bất cứ điều gì."},
	Acknowledge:        langcopy.Phrase{En: "Acknowledge it in their words before answering: %q is still open.", De: "Erkenne den Punkt in ihren Worten an, bevor du antwortest: %q ist noch offen.", Vi: "Ghi nhận bằng chính lời của họ trước khi trả lời: %q vẫn chưa được giải quyết."},
	ShowAnswer:         langcopy.Phrase{En: "The record that answers it, or the contact who can.", De: "Der Beleg, der die Frage beantwortet, oder der Kontakt, der es kann.", Vi: "Bản ghi trả lời vấn đề đó hoặc người có thể trả lời."},
	AvoidArgument:      langcopy.Phrase{En: "Re-arguing a point they have already made twice.", De: "Einen bereits zweimal vorgebrachten Punkt erneut auszudiskutieren.", Vi: "Tranh luận lại về một điểm họ đã nêu hai lần."},
	OwnDelay:           langcopy.Phrase{En: "Own the delay plainly and name a date: we owe %s on %q.", De: "Übernimm klar die Verantwortung für die Verzögerung und nenne einen Termin: Wir schulden %s etwas zu %q.", Vi: "Thẳng thắn nhận trách nhiệm về sự chậm trễ và nêu ngày cụ thể: chúng ta còn nợ %s về %q."},
	ShowReady:          langcopy.Phrase{En: "What is actually ready, and what the remaining step is.", De: "Was tatsächlich fertig ist und welcher Schritt noch fehlt.", Vi: "Điều gì thực sự đã sẵn sàng và bước nào còn lại."},
	AvoidDate:          langcopy.Phrase{En: "Promising a date nobody on our side has agreed to.", De: "Einen Termin zuzusagen, dem auf unserer Seite niemand zugestimmt hat.", Vi: "Hứa một ngày mà phía chúng ta chưa ai đồng ý."},
}
