// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var questionWords = struct {
	Intent         langcopy.Phrase
	Funding        langcopy.Phrase
	Approvers      langcopy.Phrase
	PriorContact   langcopy.Phrase
	Outstanding    langcopy.Phrase
	Background     langcopy.Phrase
	Joining        langcopy.Phrase
	Answer         langcopy.Phrase
	Progress       langcopy.Phrase
	Prepare        langcopy.Phrase
	Objection      langcopy.Phrase
	Evidence       langcopy.Phrase
	PromiseOpen    langcopy.Phrase
	RaisedOpen     langcopy.Phrase
	Changed        langcopy.Phrase
	Named          langcopy.Phrase
	ListenPriority langcopy.Phrase
	WhyIntent      langcopy.Phrase
	WhyDecision    langcopy.Phrase
	WhyDeal        langcopy.Phrase
	ListenProblem  langcopy.Phrase
	ListenRoles    langcopy.Phrase
	ListenContact  langcopy.Phrase
}{
	Intent:         langcopy.Phrase{En: "What would make this hour worth your time?", De: "Was würde diese Stunde für Sie wertvoll machen?", Vi: "Điều gì sẽ khiến khoảng thời gian này có giá trị với bạn?"},
	Funding:        langcopy.Phrase{En: "Is there a piece of work here you are trying to get funded, or is this still exploratory?", De: "Geht es um ein konkretes Vorhaben, für das Sie Budget suchen, oder noch um eine erste Orientierung?", Vi: "Bạn đang tìm ngân sách cho một công việc cụ thể hay vẫn đang tìm hiểu?"},
	Approvers:      langcopy.Phrase{En: "Who else has to agree before this can go ahead?", De: "Wer muss noch zustimmen, bevor es losgehen kann?", Vi: "Ai khác cần đồng ý trước khi việc này có thể tiến hành?"},
	PriorContact:   langcopy.Phrase{En: "Have you spoken to anyone else on our side before today?", De: "Haben Sie vor heute bereits mit jemand anderem auf unserer Seite gesprochen?", Vi: "Trước hôm nay, bạn đã trao đổi với ai khác bên chúng tôi chưa?"},
	Outstanding:    langcopy.Phrase{En: "Is anything outstanding from either side that I should pick up?", De: "Ist auf einer Seite noch etwas offen, das ich aufgreifen sollte?", Vi: "Có việc gì còn tồn đọng từ hai phía mà tôi nên tiếp nhận không?"},
	Background:     langcopy.Phrase{En: "How much of the background do you already have from your side?", De: "Wie viel vom Hintergrund kennen Sie bereits von Ihrer Seite?", Vi: "Phía bạn đã nắm được bao nhiêu thông tin nền?"},
	Joining:        langcopy.Phrase{En: "Who else is joining, and what do they need from this?", De: "Wer nimmt noch teil und was brauchen diese Personen aus dem Gespräch?", Vi: "Ai khác sẽ tham gia và họ cần gì từ cuộc trao đổi này?"},
	Answer:         langcopy.Phrase{En: "Answer it directly, or say who will and by when.", De: "Beantworte die Frage direkt oder sage, wer sie bis wann beantwortet.", Vi: "Trả lời trực tiếp hoặc cho biết ai sẽ trả lời và vào lúc nào."},
	Progress:       langcopy.Phrase{En: "Where are you with %s?", De: "Wie ist der Stand bei %s?", Vi: "Bạn đã tiến hành %s đến đâu?"},
	Prepare:        langcopy.Phrase{En: "Have the current state and the next date ready before the meeting.", De: "Halte vor dem Meeting den aktuellen Stand und den nächsten Termin bereit.", Vi: "Chuẩn bị sẵn tình trạng hiện tại và ngày tiếp theo trước cuộc họp."},
	Objection:      langcopy.Phrase{En: "%q — expect them to raise it again.", De: "%q: Rechne damit, dass sie den Punkt erneut ansprechen.", Vi: "%q — hãy chuẩn bị vì họ có thể nêu lại vấn đề này."},
	Evidence:       langcopy.Phrase{En: "Answer it with a record they can check, not with reassurance.", De: "Antworte mit einem überprüfbaren Beleg statt mit einer Beschwichtigung.", Vi: "Trả lời bằng bản ghi họ có thể kiểm tra, thay vì chỉ trấn an."},
	PromiseOpen:    langcopy.Phrase{En: "We promised %s this and it is still open.", De: "Wir haben %s dies zugesagt und es ist noch offen.", Vi: "Chúng ta đã hứa điều này với %s và vẫn chưa hoàn tất."},
	RaisedOpen:     langcopy.Phrase{En: "%s raised this and it has not been closed out.", De: "%s hat diesen Punkt angesprochen und er ist noch nicht erledigt.", Vi: "%s đã nêu vấn đề này và nó vẫn chưa được giải quyết."},
	Changed:        langcopy.Phrase{En: "You said %q — what has changed about that since?", De: "Sie haben %q gesagt. Was hat sich seitdem daran geändert?", Vi: "Bạn đã nói %q — điều gì đã thay đổi về việc đó kể từ lúc ấy?"},
	Named:          langcopy.Phrase{En: "It is the thing %s named, and the plan should be built on it rather than on what we assume.", De: "%s hat diesen Punkt benannt. Darauf sollte der Plan aufbauen, statt auf unseren Annahmen.", Vi: "Đây là điều %s đã nêu và kế hoạch nên dựa trên đó thay vì những giả định của chúng ta."},
	ListenPriority: langcopy.Phrase{En: "Whether it still matters, who owns it now, and what it is costing them.", De: "Ob es noch wichtig ist, wer jetzt verantwortlich ist und was es sie kostet.", Vi: "Liệu điều đó còn quan trọng không, ai đang chịu trách nhiệm và nó đang khiến họ tốn kém những gì."},
	WhyIntent:      langcopy.Phrase{En: "Nothing in the record says what %s wants from this meeting.", De: "Aus den erfassten Informationen geht nicht hervor, was %s von diesem Meeting erwartet.", Vi: "Không có thông tin nào trong hồ sơ cho biết %s muốn gì từ cuộc họp này."},
	WhyDecision:    langcopy.Phrase{En: "The record does not name who decides at %s.", De: "Die erfassten Informationen nennen nicht, wer bei %s entscheidet.", Vi: "Hồ sơ không nêu ai là người quyết định tại %s."},
	WhyDeal:        langcopy.Phrase{En: "There is no open deal with %s, so what this is for is unstated.", De: "Es gibt keinen offenen Deal mit %s. Der Zweck ist daher noch nicht benannt.", Vi: "Không có giao dịch đang mở với %s, nên mục đích vẫn chưa được nêu rõ."},
	ListenProblem:  langcopy.Phrase{En: "The word they use for the problem, and whether a date is attached to it.", De: "Wie sie das Problem benennen und ob es einen Termin dazu gibt.", Vi: "Từ họ dùng để mô tả vấn đề và liệu có ngày cụ thể gắn với nó không."},
	ListenRoles:    langcopy.Phrase{En: "Names and roles: who approves, who pays, who can veto.", De: "Namen und Rollen: Wer genehmigt, wer zahlt und wer kann ein Veto einlegen?", Vi: "Tên và vai trò: ai phê duyệt, ai trả tiền, ai có thể phủ quyết."},
	ListenContact:  langcopy.Phrase{En: "Whether the answer names a contact, a date, or neither.", De: "Ob die Antwort einen Kontakt, einen Termin oder keines von beiden nennt.", Vi: "Liệu câu trả lời có nêu một người liên hệ, một ngày cụ thể hay không có cả hai."},
}
