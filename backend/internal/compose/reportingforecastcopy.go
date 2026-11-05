// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var reportingForecastWords = struct{ Unpriced, MovementUnpriced langcopy.Phrase }{
	Unpriced:         langcopy.Phrase{En: "Some forecast contributions are unpriced", De: "Für einige Prognosebeiträge fehlt die Bewertung.", Vi: "Một số khoản đóng góp vào dự báo chưa được định giá."},
	MovementUnpriced: langcopy.Phrase{En: "Some captured deals could not be valued; movement covers priced contributions only", De: "Einige erfasste Deals konnten nicht bewertet werden. Die Veränderungen enthalten nur bewertete Beiträge.", Vi: "Một số deal đã ghi nhận chưa thể định giá; biến động chỉ bao gồm các khoản đóng góp đã có giá."},
}

var reportingForecastCaveats = map[forecasting.LandingCaveat]langcopy.Phrase{
	forecasting.CaveatCallAbsent:      {En: "No call recorded for this period, so this shows commit evidence.", De: "Angezeigt werden die Commit-Belege, da für diesen Zeitraum keine Einschätzung erfasst ist.", Vi: "Kỳ này chưa có nhận định nào, nên số này hiển thị bằng chứng Commit."},
	forecasting.CaveatCallBelowActual: {En: "The call is below the amount already won. It is shown as recorded, not corrected.", De: "Die Einschätzung liegt unter dem bereits gewonnenen Betrag. Die Einschätzung wird wie erfasst angezeigt, nicht korrigiert.", Vi: "Nhận định thấp hơn số tiền đã chốt. Số này hiển thị đúng như đã ghi, không chỉnh sửa."},
}

func reportingForecastReason(frame crmcontracts.ReportingContext, caveat forecasting.LandingCaveat) string {
	if caveat == "" {
		return reporting.ExplanationCopy(frame).Say(reportingForecastWords.Unpriced)
	}
	if phrase, ok := reportingForecastCaveats[caveat]; ok {
		return reporting.ExplanationCopy(frame).Say(phrase)
	}
	return string(caveat)
}
