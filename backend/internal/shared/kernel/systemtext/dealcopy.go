// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var dealDefinitions = map[string]Definition{
	"deals.draftFollowUp":      {Words: langcopy.Phrase{En: "Draft a follow-up on %q — the %s on %s left no next step planned", De: "Entwirf ein Follow-up zu %q: Nach dem Austausch (%s) am %s ist kein nächster Schritt geplant.", Vi: "Soạn một follow-up cho %q: sau tương tác (%s) vào ngày %s chưa có bước tiếp theo nào được lên kế hoạch."}, Arguments: []string{argumentText, argumentText, argumentText}},
	"deals.followUpSubject":    {Words: langcopy.Phrase{En: "Follow up on %s", De: "Follow-up zu %s", Vi: "Follow-up cho %s"}, Arguments: []string{argumentText}},
	"deals.followUpBody":       {Words: langcopy.Phrase{En: "Follow up on the %s from %s. No next step is on the timeline yet.", De: "Follow-up zum Austausch (%s) vom %s. Im Verlauf steht noch kein nächster Schritt.", Vi: "Follow-up cho tương tác (%s) vào ngày %s. Dòng thời gian chưa có bước tiếp theo nào."}, Arguments: []string{argumentText, argumentText}},
	"deals.followUpBodyQuoted": {Words: langcopy.Phrase{En: "Follow up on the %s “%s” from %s. No next step is on the timeline yet.", De: "Follow-up zum Austausch (%s) „%s“ vom %s. Im Verlauf steht noch kein nächster Schritt.", Vi: "Follow-up cho tương tác (%s) “%s” vào ngày %s. Dòng thời gian chưa có bước tiếp theo nào."}, Arguments: []string{argumentText, argumentText, argumentText}},
	"deal.opened":              {Words: langcopy.Phrase{En: "Margince opened a deal for %s", De: "Margince hat einen Deal für %s angelegt", Vi: "Margince đã mở deal cho %s"}, Arguments: []string{argumentText}},
	"deal.opened_body":         {Words: langcopy.Phrase{En: "%s. Undo it from Home if this is not a deal.", De: "%s. Mache dies auf der Startseite rückgängig, falls es kein Deal ist.", Vi: "%s. Hoàn tác trên Trang chủ nếu đây không phải deal."}, Arguments: []string{argumentText}},
	"deal.reopened":            {Words: langcopy.Phrase{En: "Margince reopened %s", De: "Margince hat %s wieder geöffnet", Vi: "Margince đã mở lại %s"}, Arguments: []string{argumentText}},
	"deal.reopened_body":       {Words: langcopy.Phrase{En: "New buying evidence arrived at the company after this deal was lost. Undo it from Home if the deal is still lost.", De: "Nach dem Verlust dieses Deals sind neue Kaufhinweise beim Unternehmen eingegangen. Mache dies auf der Startseite rückgängig, falls der Deal weiterhin verloren ist.", Vi: "Có bằng chứng mua hàng mới tại công ty sau khi deal này đã thua. Hoàn tác trên Trang chủ nếu deal vẫn thua."}, Arguments: []string{}},
	"deal.lost":                {Words: langcopy.Phrase{En: "Margince closed this deal as lost", De: "Margince hat diesen Deal als verloren geschlossen", Vi: "Margince đã đóng deal này là thua"}, Arguments: []string{}},
	"deal.customer_declined":   {Words: langcopy.Phrase{En: "The customer wrote that they will not go ahead", De: "Der Kunde hat geschrieben, dass er nicht fortfahren wird", Vi: "Khách hàng đã viết rằng họ sẽ không tiếp tục"}, Arguments: []string{}},
	"deal.silent":              {Words: langcopy.Phrase{En: "No word from the customer for %d days", De: "Seit %d Tagen keine Nachricht vom Kunden", Vi: "Chưa có tin từ khách hàng trong %d ngày"}, Arguments: []string{argumentCount}},
	"deal.silent_threshold":    {Words: langcopy.Phrase{En: "No word from the customer for more than %d days", De: "Seit mehr als %d Tagen keine Nachricht vom Kunden", Vi: "Chưa có tin từ khách hàng trong hơn %d ngày"}, Arguments: []string{argumentCount}},
	"deal.lost_undo":           {Words: langcopy.Phrase{En: "Undo it from Home if the deal is still alive, and add a note on why.", De: "Mache dies auf der Startseite rückgängig, falls der Deal noch aktiv ist, und begründe es in einer Notiz.", Vi: "Hoàn tác trên Trang chủ nếu deal vẫn còn cơ hội và thêm ghi chú giải thích lý do."}, Arguments: []string{}},
	"deal.lost_refusal_linked": {Words: langcopy.Phrase{En: "The message Margince read as a no is linked under Done for you on Home.", De: "Die Nachricht, die Margince als Absage gelesen hat, ist auf der Startseite unter „Für dich erledigt“ verlinkt.", Vi: "Tin nhắn mà Margince hiểu là lời từ chối được liên kết trong mục Margince đã làm giúp bạn trên Trang chủ."}, Arguments: []string{}},
	"deal.updated":             {Words: langcopy.Phrase{En: "Margince updated this deal", De: "Margince hat diesen Deal aktualisiert", Vi: "Margince đã cập nhật deal này"}, Arguments: []string{}},
	"deal.verdict_date":        {Words: langcopy.Phrase{En: "Close date set to %s", De: "Abschlussdatum auf %s gesetzt", Vi: "Ngày chốt được đặt thành %s"}, Arguments: []string{argumentDate}},
	"deal.verdict_stage":       {Words: langcopy.Phrase{En: "Moved to %s", De: "In Phase %s verschoben", Vi: "Đã chuyển sang giai đoạn %s"}, Arguments: []string{argumentText}},
	"deal.verdict_amount":      {Words: langcopy.Phrase{En: "Amount set to %s %s", De: "Betrag auf %s %s gesetzt", Vi: "Số tiền được đặt thành %s %s"}, Arguments: []string{argumentText, argumentText}},
	"deal.verdict_probability": {Words: langcopy.Phrase{En: "Win probability set to %d%%", De: "Gewinnwahrscheinlichkeit auf %d%% gesetzt", Vi: "Xác suất thắng được đặt thành %d%%"}, Arguments: []string{argumentCount}},
	"deal.verdict_basis":       {Words: langcopy.Phrase{En: labeledPair, De: labeledPair, Vi: labeledPair}, Arguments: []string{argumentText, argumentText}},
	"deal.verdict_undo":        {Words: langcopy.Phrase{En: "Undo any of these from Home.", De: "Mache diese Änderungen auf der Startseite rückgängig.", Vi: "Hoàn tác các thay đổi này trên Trang chủ."}, Arguments: []string{}},
}
