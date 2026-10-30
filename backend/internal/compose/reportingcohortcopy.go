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
	cohortHistoryPartial: {En: "Direction not recorded", De: "Richtung nicht erfasst", Vi: "Chưa ghi nhận hướng chuyển"},
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

var cohortWords = struct{ Arrivals, StageOrderUnknown, ArrivalOwnerUnknown langcopy.Phrase }{
	Arrivals:            langcopy.Phrase{En: "Leads that arrived", De: "Eingegangene Leads", Vi: "Lead đã đến"},
	StageOrderUnknown:   langcopy.Phrase{En: "Some deals left a stage before Margince kept the stage order with each move, so whether they moved forward or back is not known. They are counted apart and left out of the rate.", De: "Einige Deals haben eine Phase verlassen, bevor Margince bei jedem Wechsel die Reihenfolge der Phasen gespeichert hat. Ob sie vor- oder zurückgegangen sind, ist daher nicht bekannt. Sie werden getrennt gezählt und fließen nicht in die Quote ein.", Vi: "Một số deal rời giai đoạn trước khi Margince lưu thứ tự giai đoạn cùng mỗi lần chuyển, nên không biết chúng tiến lên hay lùi lại. Các deal này được đếm riêng và không tính vào tỷ lệ."},
	ArrivalOwnerUnknown: langcopy.Phrase{En: "Some leads arrived before Margince kept who owned a lead when it arrived. They count for the whole workspace, but for no person or team.", De: "Einige Leads sind eingegangen, bevor Margince gespeichert hat, wem ein Lead bei seinem Eingang gehörte. Sie zählen für den gesamten Workspace, aber für keine Person und kein Team.", Vi: "Một số lead đến trước khi Margince lưu người phụ trách lead tại thời điểm lead đến. Các lead này được tính cho toàn bộ không gian làm việc, nhưng không tính cho người hay nhóm nào."},
}
