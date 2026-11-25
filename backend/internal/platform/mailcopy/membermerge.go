// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// MergeConfirmMessage builds the confirmation request from its link.
func (c Copy) MergeConfirmMessage(goes, stays, link string) emailview.Message {
	var content emailview.Content
	content.Lead(c.MergeIntro)
	content.Paragraph(fmt.Sprintf(c.MergeWhich, OneLine(goes), OneLine(stays)))
	content.Action(c.MergeAction, link)
	content.Footnote(c.MergeIgnore)
	return content.Message(c.MergeSubject, string(c.Language), c.Preview(c.MergeSubject))
}

func mergeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MergeSubject },
		"Confirm that two Margince accounts may be merged",
		"Bestätige, dass zwei Margince-Konten zusammengeführt werden dürfen",
		"Xác nhận gộp hai tài khoản Margince")
	line(func(c *Copy) *string { return &c.MergeIntro },
		"An administrator asked to merge two Margince accounts that both belong to you. The private mail and activity of the account that goes away move to the account that stays. Confirm only if both accounts are yours. The link is valid for three days and works once.",
		"Ein Admin möchte zwei Margince-Konten zusammenführen, die beide dir gehören. Private E-Mails und Aktivitäten des Kontos, das entfällt, wandern in das Konto, das bleibt. Bestätige nur, wenn beide Konten dir gehören. Der Link ist drei Tage gültig und funktioniert einmal.",
		"Quản trị viên đề nghị gộp hai tài khoản Margince đều của bạn. Thư và hoạt động riêng của tài khoản bị bỏ sẽ chuyển sang tài khoản được giữ lại. Chỉ xác nhận nếu cả hai tài khoản đều của bạn. Liên kết có hiệu lực trong ba ngày và chỉ dùng được một lần.")
	line(func(c *Copy) *string { return &c.MergeWhich },
		"%s goes away and %s stays.",
		"%s entfällt und %s bleibt.",
		"%s sẽ bị bỏ và %s được giữ lại.")
	line(func(c *Copy) *string { return &c.MergeAction },
		"Confirm the merge",
		"Zusammenführung bestätigen",
		"Xác nhận gộp")
	line(func(c *Copy) *string { return &c.MergeIgnore },
		"If you did not expect this, ignore this email. Nothing changes, and the link signs nobody in.",
		"Wenn du das nicht erwartet hast, ignoriere diese E-Mail. Es ändert sich nichts, und der Link meldet niemanden an.",
		"Nếu bạn không chờ email này, hãy bỏ qua. Không có gì thay đổi và liên kết này không đăng nhập cho ai.")
	line(func(c *Copy) *string { return &c.MergePreview },
		"Say whether two of your accounts may become one.",
		"Sag, ob zwei deiner Konten zu einem werden dürfen.",
		"Cho biết hai tài khoản của bạn có được gộp thành một hay không.")
}
