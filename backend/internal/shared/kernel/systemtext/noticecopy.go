// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var noticeDefinitions = map[string]Definition{
	"receipt.overdue": {Words: langcopy.Phrase{En: "This refusal's receipt was due by %s and has not been delivered. Delivery status: %s. The refusal itself is recorded and stays in force.", De: "Die Bestätigung dieser Ablehnung war bis %s fällig und wurde nicht zugestellt. Zustellstatus: %s. Die Ablehnung ist erfasst und bleibt wirksam.", Vi: "Xác nhận từ chối này đến hạn vào %s nhưng chưa được gửi. Trạng thái gửi: %s. Việc từ chối đã được ghi nhận và vẫn có hiệu lực."}, Arguments: []string{argumentDate, argumentText}},

	"privacy.review.inactive": {Words: langcopy.Phrase{En: "An inactive member's retained signature needs manual privacy review. Signature reference: %s. Member reference: %s. An administrator must review it before that member is reactivated.", De: "Die gespeicherte Signatur eines inaktiven Mitglieds erfordert eine manuelle Datenschutzprüfung. Signaturreferenz: %s. Mitgliedsreferenz: %s. Ein Administrator muss sie vor der Reaktivierung des Mitglieds prüfen.", Vi: "Chữ ký được lưu giữ của thành viên không hoạt động cần được kiểm tra quyền riêng tư thủ công. Mã chữ ký: %s. Mã thành viên: %s. Quản trị viên phải kiểm tra trước khi kích hoạt lại thành viên."}, Arguments: []string{argumentText, argumentText}},

	"notice.NoticeSLASubject":     {Words: langcopy.Phrase{En: "SLA breach — first response overdue", De: "SLA-Verstoß — erste Antwort überfällig", Vi: "Vi phạm SLA — phản hồi đầu tiên đã quá hạn"}, Arguments: []string{}},
	"notice.NoticeSLABody":        {Words: langcopy.Phrase{En: "A lead's first response is overdue; its escalation task is on your list.", De: "Die erste Antwort auf einen Lead ist überfällig; die Eskalationsaufgabe steht auf deiner Liste.", Vi: "Phản hồi đầu tiên cho một lead đã quá hạn; công việc leo thang nằm trong danh sách của bạn."}, Arguments: []string{}},
	"notice.NoticeBacklogSubject": {Words: langcopy.Phrase{En: "Some captured emails are not yet available to your team", De: "Einige erfasste E-Mails sind für dein Team noch nicht verfügbar", Vi: "Một số email đã nhận chưa được hiển thị cho nhóm"}, Arguments: []string{}},
	"notice.NoticeBacklogBody":    {Words: langcopy.Phrase{En: "Senders awaiting review: %d. Your team cannot see these messages yet. The checks determine which messages can be shared with your team.", De: "Absender mit ausstehender Prüfung: %d. Dein Team kann diese Nachrichten noch nicht sehen. Nach der Prüfung zeigt Margince die Nachrichten an, die für dein Team freigegeben sind.", Vi: "Người gửi đang chờ kiểm tra: %d. Nhóm chưa thể xem các thư này. Việc kiểm tra xác định thư nào có thể được chia sẻ với nhóm."}, Arguments: []string{argumentCount}},
}
