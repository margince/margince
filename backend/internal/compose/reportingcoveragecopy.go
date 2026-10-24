// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

//nolint:dupl // a copy table has the shape of every other copy table, and no text in common
var reportingCoverageWords = struct{ Qualification, AcceptancePipeline, MeetingPipeline, LegacyMeetings, LegacyHistory, QualificationHistory, ChoosePipeline, ForecastAccess, MovementScope, RestrictedMovement, SingleTeam, CalculationFailed, NoPipeline langcopy.Phrase }{
	Qualification:        langcopy.Phrase{En: "Choose qualifying stages in Reporting setup", De: "Wähle in den Berichtseinstellungen die qualifizierenden Phasen aus.", Vi: "Chọn các giai đoạn đủ điều kiện trong phần thiết lập báo cáo."},
	AcceptancePipeline:   langcopy.Phrase{En: "Acceptance credit has no historical pipeline dimension; choose all pipelines", De: "Die Anerkennung angenommener Verkaufschancen enthält keine historische Pipeline-Zuordnung. Wähle alle Pipelines aus.", Vi: "Ghi nhận cơ hội được chấp nhận không có dữ liệu phễu lịch sử; hãy chọn tất cả phễu."},
	MeetingPipeline:      langcopy.Phrase{En: "Meetings have no pipeline dimension; choose all pipelines", De: "Besprechungen sind keiner Pipeline zugeordnet. Wähle alle Pipelines aus.", Vi: "Cuộc họp không được phân theo phễu; hãy chọn tất cả phễu."},
	LegacyMeetings:       langcopy.Phrase{En: "Older meetings use their current host and customer links because historical attribution is unavailable.", De: "Bei älteren Besprechungen werden die aktuellen Gastgeber und Kundenverknüpfungen verwendet, da die historische Zuordnung fehlt.", Vi: "Các cuộc họp cũ dùng người chủ trì và liên kết khách hàng hiện tại vì không có dữ liệu phân bổ lịch sử."},
	LegacyHistory:        langcopy.Phrase{En: "Some source history predates reliable attribution. Only known contributions are shown; this period is not a complete comparison baseline.", De: "Ein Teil der Historie stammt aus der Zeit vor der verlässlichen Zuordnung. Angezeigt werden nur bekannte Beiträge. Dieser Zeitraum ist keine vollständige Vergleichsbasis.", Vi: "Một phần lịch sử có trước thời điểm phân bổ đáng tin cậy. Chỉ hiển thị các khoản đóng góp đã biết; kỳ này không phải cơ sở so sánh đầy đủ."},
	QualificationHistory: langcopy.Phrase{En: "Qualification tracking began after part of this period. Only transitions recorded under a configured stage framework are included.", De: "Die Erfassung der Qualifizierung begann erst im Verlauf dieses Zeitraums. Enthalten sind nur Übergänge, die unter einer eingerichteten Phasenzuordnung erfasst wurden.", Vi: "Việc theo dõi đủ điều kiện bắt đầu sau một phần của kỳ này. Chỉ bao gồm các chuyển đổi được ghi nhận theo cấu hình giai đoạn đã thiết lập."},
	ChoosePipeline:       langcopy.Phrase{En: "Choose one pipeline to compare stages", De: "Wähle eine Pipeline aus, um Phasen zu vergleichen.", Vi: "Chọn một phễu để so sánh các giai đoạn."},
	ForecastAccess:       langcopy.Phrase{En: "Forecast access is required for captured movement", De: "Für erfasste Veränderungen ist der Zugriff auf Prognosen erforderlich.", Vi: "Cần quyền truy cập dự báo để xem biến động đã ghi nhận."},
	MovementScope:        langcopy.Phrase{En: "Choose a fixed population and fiscal-quarter close window for movement", De: "Wähle für die Veränderungen einen festen Personenkreis und ein Abschlussfenster innerhalb eines Geschäftsquartals aus.", Vi: "Chọn nhóm cố định và khoảng chốt theo quý tài chính để xem biến động."},
	RestrictedMovement:   langcopy.Phrase{En: "Some captured contributions are restricted", De: "Einige erfasste Beiträge sind zugriffsbeschränkt.", Vi: "Một số khoản đóng góp đã ghi nhận bị hạn chế truy cập."},
	SingleTeam:           langcopy.Phrase{En: "Choose a single team for its forecast", De: "Wähle ein einzelnes Team aus, um seine Prognose zu sehen.", Vi: "Chọn một nhóm để xem dự báo của nhóm đó."},
	CalculationFailed:    langcopy.Phrase{En: "This figure could not be calculated just now. The other figures are not affected; reload to try again.", De: "Diese Zahl konnte gerade nicht berechnet werden. Die anderen Zahlen sind nicht betroffen. Lade die Seite neu, um es noch einmal zu versuchen.", Vi: "Không thể tính số liệu này lúc này. Các số liệu khác không bị ảnh hưởng; hãy tải lại để thử lại."},
	NoPipeline:           langcopy.Phrase{En: "This figure is not split by pipeline; choose all pipelines", De: "Diese Zahl wird nicht nach Pipeline aufgeteilt. Wähle alle Pipelines aus.", Vi: "Số liệu này không chia theo pipeline; hãy chọn tất cả pipeline."},
}
