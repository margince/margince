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
		"Phản hồi đầu tiên cho một lead đã quá hạn; công việc leo thang nằm trong danh sách của bạn.")
	line(func(c *Copy) *string { return &c.NoticeBacklogSubject },
		"Some captured emails are not yet available to your team",
		"Einige erfasste E-Mails sind für dein Team noch nicht verfügbar",
		"Một số email đã nhận chưa được hiển thị cho nhóm")
	// The count leads as a label rather than a quantity in a sentence.
	//
	// One waiting decision is reachable, because the seats query counts and
	// groups with no lower bound. A sentence would read "1 sender decisions".
	// Vietnamese does not inflect for number and reads naturally either way.
	line(func(c *Copy) *string { return &c.NoticeBacklogBody },
		"Senders awaiting review: %d. Your team cannot see these messages yet. The checks determine which messages can be shared with your team.",
		"Absender mit ausstehender Prüfung: %d. Dein Team kann diese Nachrichten noch nicht sehen. Nach der Prüfung zeigt Margince die Nachrichten an, die für dein Team freigegeben sind.",
		"Người gửi đang chờ kiểm tra: %d. Nhóm chưa thể xem các thư này. Việc kiểm tra xác định thư nào có thể được chia sẻ với nhóm.")
}
