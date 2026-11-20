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
	cohortHistoryPartial: {En: "Next move imported, synced or unknown", De: "Nächster Wechsel importiert, synchronisiert oder unbekannt", Vi: "Lần chuyển tiếp theo được nhập, đồng bộ hoặc không rõ"},
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
