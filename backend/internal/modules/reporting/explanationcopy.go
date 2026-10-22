// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// ExplanationCopy preserves the recorded language, with English for older editions.
func ExplanationCopy(frame crmcontracts.ReportingContext) langcopy.Spoken {
	language := "en"
	if frame.ProseLanguage != nil {
		language = *frame.ProseLanguage
	}
	return langcopy.For(language)
}

var comparisonWords = struct{ Restricted, Report, Population, Context, Selection, Timezone, Period, Metric langcopy.Phrase }{
	Restricted: langcopy.Phrase{En: "Historical evidence is restricted or redacted", De: "Historische Belege sind zugriffsbeschränkt oder geschwärzt", Vi: "Bằng chứng lịch sử bị hạn chế truy cập hoặc đã được che"},
	Report:     langcopy.Phrase{En: "Choose two editions of the same report", De: "Wähle zwei Ausgaben desselben Berichts", Vi: "Chọn hai bản của cùng một báo cáo"},
	Population: langcopy.Phrase{En: "Team membership or population changed", De: "Die Teammitglieder oder die ausgewertete Gruppe haben sich geändert", Vi: "Thành viên nhóm hoặc tập đối tượng đã thay đổi"},
	Context:    langcopy.Phrase{En: "Reporting definitions or context differ", De: "Die Berichtsdefinitionen oder der Auswertungskontext unterscheiden sich", Vi: "Định nghĩa hoặc bối cảnh báo cáo khác nhau"},
	Selection:  langcopy.Phrase{En: "Saved report selections differ", De: "Die gespeicherten Berichtsauswahlen unterscheiden sich", Vi: "Các lựa chọn đã lưu của báo cáo khác nhau"},
	Timezone:   langcopy.Phrase{En: "Reporting timezone is unavailable", De: "Die Zeitzone des Berichts ist nicht verfügbar", Vi: "Múi giờ báo cáo không khả dụng"},
	Period:     langcopy.Phrase{En: "Compare adjacent complete weeks, months or quarters", De: "Vergleiche aufeinanderfolgende vollständige Wochen, Monate oder Quartale", Vi: "So sánh các tuần, tháng hoặc quý đầy đủ liên tiếp"},
	Metric:     langcopy.Phrase{En: "Metric versions differ", De: "Die Kennzahlenversionen unterscheiden sich", Vi: "Phiên bản chỉ số khác nhau"},
}
