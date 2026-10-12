// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The worklist notices the product raises for itself.
//
// Its own file rather than another section of catalog.go, which is at the
// length a reader can hold. The catalog's sections are one function per
// message for the same reason: a translator opens the section they are
// working on.

// noticeLines is what a worklist notice the product raises says.
func noticeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.NoticeSLASubject },
		"SLA breach — first response overdue",
		"SLA-Verstoß — erste Antwort überfällig",
		"Vi phạm SLA — phản hồi đầu tiên đã quá hạn")
	line(func(c *Copy) *string { return &c.NoticeSLABody },
		"A lead's first response is overdue; its escalation task is on your list.",
		"Die erste Antwort auf einen Lead ist überfällig; die Eskalationsaufgabe steht auf deiner Liste.",
		"Phản hồi đầu tiên cho một khách hàng tiềm năng đã quá hạn; công việc leo thang nằm trong danh sách của bạn.")
	line(func(c *Copy) *string { return &c.NoticeBacklogSubject },
		"Your captured mail is waiting on the classifier",
		"Deine erfasste Mail wartet auf die Klassifizierung",
		"Thư đã thu của bạn đang chờ bộ phân loại")
	// The count leads as a label rather than a quantity in a sentence.
	//
	// One waiting decision is reachable, because the seats query counts and
	// groups with no lower bound. A sentence would read "1 sender decisions".
	// Vietnamese does not inflect for number and reads naturally either way.
	line(func(c *Copy) *string { return &c.NoticeBacklogBody },
		"Sender decisions waiting without an answer: %d. Mail stays withheld from "+
			"your colleagues while this lasts — nothing is lost, and it clears on its own "+
			"once the classifier answers again.",
		"Absender-Entscheidungen ohne Antwort: %d. Mail bleibt deinen Kolleginnen und Kollegen "+
			"vorenthalten, solange das anhält — nichts geht verloren, und es löst sich von selbst, "+
			"sobald die Klassifizierung wieder antwortet.",
		"%d quyết định về người gửi đang chờ mà chưa có câu trả lời. Thư vẫn được giữ lại, "+
			"không cho đồng nghiệp của bạn thấy, trong thời gian này — không có gì bị mất, "+
			"và tình trạng này tự hết khi bộ phân loại trả lời lại.")
}
