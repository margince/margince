// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

// cohortOutcomeWords name what became of a cohort member, as a stage or lead
// cohort point lists it.
var cohortOutcomeWords = map[string]langcopy.Phrase{
	cohortAdvanced:       {En: "Moved to a later stage", De: "In eine spätere Phase gewechselt", Vi: "Đã sang giai đoạn sau"},
	cohortWon:            {En: "Won", De: "Gewonnen", Vi: "Đã thắng"},
	cohortLost:           {En: "Lost", De: "Verloren", Vi: "Đã thua"},
	cohortRegressed:      {En: "Moved back", De: "In eine frühere Phase zurück", Vi: "Đã lùi về giai đoạn trước"},
	cohortLeftPipeline:   {En: "Moved to another pipeline", De: "In eine andere Pipeline gewechselt", Vi: "Đã chuyển sang pipeline khác"},
	cohortUnresolved:     {En: "Still open", De: "Noch offen", Vi: "Vẫn đang mở"},
	cohortHistoryPartial: {En: "Next move imported or unknown", De: "Nächster Wechsel importiert oder unbekannt", Vi: "Lần chuyển tiếp theo được nhập hoặc không rõ"},
	cohortAccepted:       {En: "Handoff accepted", De: "Übergabe angenommen", Vi: "Bàn giao đã được chấp nhận"},
	cohortQualified:      {En: "Qualified", De: "Qualifiziert", Vi: "Đủ điều kiện"},
	cohortDisqualified:   {En: "Disqualified", De: "Disqualifiziert", Vi: "Đã loại"},
}

// cohortEntryWords name how a deal entered the stage it is followed from.
var cohortEntryWords = map[string]langcopy.Phrase{
	cohortEntryFirst:    {En: "First time in this stage", De: "Zum ersten Mal in dieser Phase", Vi: "Lần đầu vào giai đoạn này"},
	cohortEntryRepeat:   {En: "Back in this stage", De: "Erneut in dieser Phase", Vi: "Quay lại giai đoạn này"},
	cohortEntryReopened: {En: "Reopened after a close", De: "Nach einem Abschluss wieder geöffnet", Vi: "Mở lại sau khi đã đóng"},
}

var cohortWords = struct{ Arrivals, ExitNotRecorded, ArrivalOwnerUnknown langcopy.Phrase }{
	Arrivals:            langcopy.Phrase{En: "Leads that arrived", De: "Eingegangene Leads", Vi: "Lead đã đến"},
	ExitNotRecorded:     langcopy.Phrase{En: "Some deals left a stage by a move an import brought in, which does not show how your team sells, or by a move saved before Margince kept the stage order. These deals are counted apart and left out of the rate.", De: "Einige Deals haben eine Phase durch einen Wechsel verlassen, den ein Import mitgebracht hat und der nicht zeigt, wie dein Team verkauft, oder durch einen Wechsel, der gespeichert wurde, bevor Margince die Reihenfolge der Phasen festgehalten hat. Diese Deals werden getrennt gezählt und fließen nicht in die Quote ein.", Vi: "Một số deal rời giai đoạn bằng một lần chuyển do lần nhập dữ liệu mang vào, vốn không cho thấy cách nhóm của bạn bán hàng, hoặc bằng một lần chuyển được lưu trước khi Margince ghi thứ tự giai đoạn. Các deal này được đếm riêng và không tính vào tỷ lệ."},
	ArrivalOwnerUnknown: langcopy.Phrase{En: "Some leads arrived before Margince kept who owned a lead when it arrived. They count for the whole workspace, but for no team member or team.", De: "Einige Leads sind eingegangen, bevor Margince gespeichert hat, wem ein Lead bei seinem Eingang gehörte. Sie zählen für den gesamten Workspace, aber für kein Teammitglied und kein Team.", Vi: "Một số lead đến trước khi Margince lưu người phụ trách lead tại thời điểm lead đến. Các lead này được tính cho toàn bộ không gian làm việc, nhưng không tính cho người hay nhóm nào."},
}
