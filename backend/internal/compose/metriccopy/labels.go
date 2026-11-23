// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package metriccopy

import (
	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// Labels name a reporting row or group that has no name of its own.
var Labels = struct{ OwnerMissing, Unassigned, ForecastContribution, PipelineContribution, Unmapped, Meeting, Introduction, MetricRetired langcopy.Phrase }{
	PipelineContribution: langcopy.Phrase{En: "Captured pipeline contribution", De: "Erfasster Pipelinebeitrag", Vi: "Khoản đóng góp vào pipeline đã ghi nhận"},
	Unmapped:             langcopy.Phrase{En: "Unmapped", De: "Nicht zugeordnet", Vi: "Chưa ánh xạ"},
	Meeting:              langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"},
	Introduction:         langcopy.Phrase{En: "Introduction", De: "Vorstellung", Vi: "Lời giới thiệu"},
	MetricRetired:        langcopy.Phrase{En: "Metric no longer available", De: "Kennzahl nicht mehr verfügbar", Vi: "Chỉ số không còn khả dụng"},
	OwnerMissing:         langcopy.Phrase{En: "Owner not recorded", De: "Verantwortliche Person nicht erfasst", Vi: "Chưa ghi nhận người chịu trách nhiệm"},
	Unassigned:           langcopy.Phrase{En: "Unassigned", De: "Nicht zugewiesen", Vi: "Chưa phân công"},
	ForecastContribution: langcopy.Phrase{En: "Forecast contribution", De: "Prognosebeitrag", Vi: "Khoản đóng góp vào dự báo"},
}

// BreakdownLabels name the groups a lead or partner breakdown splits a
// figure into. A lead status reads as the lead page shows it.
var BreakdownLabels = struct{ NoOrigin, Lead, PartnerHidden, Sourced, Influenced, Active, Inactive, CompanyAccess langcopy.Phrase }{
	NoOrigin:      langcopy.Phrase{En: "No origin", De: "Keine Herkunft", Vi: "Chưa có nguồn gốc"},
	Lead:          langcopy.Phrase{En: "Unnamed lead", De: "Lead ohne Namen", Vi: "Lead chưa có tên"},
	PartnerHidden: langcopy.Phrase{En: "Partner you cannot see", De: "Nicht sichtbares Partnerunternehmen", Vi: "Đối tác bạn không xem được"},
	Sourced:       langcopy.Phrase{En: "Sourced", De: "Vermittelt", Vi: "Mang deal về"},
	Influenced:    langcopy.Phrase{En: "Influenced", De: "Beeinflusst", Vi: "Hỗ trợ deal"},
	Active:        langcopy.Phrase{En: "Active", De: "Aktiv", Vi: "Đang hoạt động"},
	Inactive:      langcopy.Phrase{En: "Inactive", De: "Inaktiv", Vi: "Không hoạt động"},
	CompanyAccess: langcopy.Phrase{En: "Partners are counted only for readers who may see companies", De: "Partnerunternehmen werden nur gezählt, wenn du Unternehmen sehen darfst.", Vi: "Chỉ đếm đối tác khi bạn được phép xem công ty."},
}

// LeadStatusLabels name the status a lead moved to, as the lead page shows it.
var LeadStatusLabels = map[crmcontracts.LeadStatus]langcopy.Phrase{
	crmcontracts.LeadStatusContacted:    {En: "Contacted", De: "Kontaktiert", Vi: "Đã liên hệ"},
	crmcontracts.LeadStatusEngaged:      {En: "Engaged", De: "Im Gespräch", Vi: "Đang trao đổi"},
	crmcontracts.LeadStatusPromoted:     {En: "Qualified", De: "Qualifiziert", Vi: "Đủ điều kiện"},
	crmcontracts.LeadStatusDisqualified: {En: "Disqualified", De: "Disqualifiziert", Vi: "Đã loại"},
}

// Coverage explains why a figure is partial, refused or missing.
//
//nolint:dupl // a copy table has the shape of every other copy table, and no text in common
var Coverage = struct{ Qualification, AcceptancePipeline, MeetingPipeline, LegacyMeetings, LegacyMeetingDay, LegacyHistory, LegacyStageMoves, QualificationHistory, ChoosePipeline, ForecastAccess, MovementScope, RestrictedMovement, SingleTeam, CalculationFailed, NoPipeline, IndividualOff, BelowFloor, Dimension, ValueBasisOpen, ChooseInitiative langcopy.Phrase }{
	Qualification:        langcopy.Phrase{En: "Choose qualifying stages in Reporting setup", De: "Wähle in den Berichtseinstellungen die qualifizierenden Phasen aus.", Vi: "Chọn các giai đoạn đủ điều kiện trong phần thiết lập báo cáo."},
	AcceptancePipeline:   langcopy.Phrase{En: "Acceptance credit has no historical pipeline dimension; choose all pipelines", De: "Die Anerkennung angenommener Verkaufschancen enthält keine historische Pipeline-Zuordnung. Wähle alle Pipelines aus.", Vi: "Ghi nhận cơ hội được chấp nhận không có dữ liệu phễu lịch sử; hãy chọn tất cả phễu."},
	MeetingPipeline:      langcopy.Phrase{En: "Meetings have no pipeline dimension; choose all pipelines", De: "Besprechungen sind keiner Pipeline zugeordnet. Wähle alle Pipelines aus.", Vi: "Cuộc họp không được phân theo phễu; hãy chọn tất cả phễu."},
	LegacyMeetings:       langcopy.Phrase{En: "Older meetings use their current host and customer links because historical attribution is unavailable.", De: "Bei älteren Besprechungen werden die aktuellen Gastgeber und Kundenverknüpfungen verwendet, da die historische Zuordnung fehlt.", Vi: "Các cuộc họp cũ dùng người chủ trì và liên kết khách hàng hiện tại vì không có dữ liệu phân bổ lịch sử."},
	LegacyMeetingDay:     langcopy.Phrase{En: "Some older meetings were recorded before Margince kept whether their start had a time, so a meeting entered as a date may count on the day next to it.", De: "Einige ältere Besprechungen wurden erfasst, bevor Margince speicherte, ob ihr Beginn eine Uhrzeit hatte. Eine Besprechung, die nur mit Datum eingetragen wurde, kann daher am Nachbartag zählen.", Vi: "Một số cuộc họp cũ được ghi lại trước khi Margince lưu việc thời điểm bắt đầu có giờ hay không, nên một cuộc họp chỉ nhập ngày có thể được tính vào ngày liền kề."},
	LegacyHistory:        langcopy.Phrase{En: "Some source history predates reliable attribution. Only known contributions are shown; this period is not a complete comparison baseline.", De: "Ein Teil der Historie stammt aus der Zeit vor der verlässlichen Zuordnung. Angezeigt werden nur bekannte Beiträge. Dieser Zeitraum ist keine vollständige Vergleichsbasis.", Vi: "Một phần lịch sử có trước thời điểm phân bổ đáng tin cậy. Chỉ hiển thị các khoản đóng góp đã biết; kỳ này không phải cơ sở so sánh đầy đủ."},
	LegacyStageMoves:     langcopy.Phrase{En: "Some moves in this period were saved before Margince kept everything a move needs: the owner, the pipeline, or whether an import brought it in. They are counted as moves made here, though some may be imports, so this period is not a complete comparison baseline.", De: "Einige Wechsel in diesem Zeitraum wurden gespeichert, bevor Margince alles zu einem Wechsel festgehalten hat: den Verantwortlichen, die Pipeline oder ob ein Import ihn mitgebracht hat. Sie zählen als Wechsel in Margince, auch wenn einige davon Importe sein können. Dieser Zeitraum ist daher keine vollständige Vergleichsbasis.", Vi: "Một số lần chuyển trong kỳ này được lưu trước khi Margince ghi đủ thông tin của một lần chuyển: người phụ trách, pipeline, hoặc lần chuyển đó có do nhập dữ liệu mang vào hay không. Chúng được tính là lần chuyển thực hiện trong Margince, dù một số có thể là dữ liệu nhập, nên kỳ này không phải cơ sở so sánh đầy đủ."},
	QualificationHistory: langcopy.Phrase{En: "Qualification tracking began after part of this period. Only transitions recorded under a configured stage framework are included.", De: "Die Erfassung der Qualifizierung begann erst im Verlauf dieses Zeitraums. Enthalten sind nur Übergänge, die unter einer eingerichteten Phasenzuordnung erfasst wurden.", Vi: "Việc theo dõi đủ điều kiện bắt đầu sau một phần của kỳ này. Chỉ bao gồm các chuyển đổi được ghi nhận theo cấu hình giai đoạn đã thiết lập."},
	ChoosePipeline:       langcopy.Phrase{En: "Choose one pipeline to compare stages", De: "Wähle eine Pipeline aus, um Phasen zu vergleichen.", Vi: "Chọn một phễu để so sánh các giai đoạn."},
	ForecastAccess:       langcopy.Phrase{En: "Forecast access is required for captured movement", De: "Für erfasste Veränderungen ist der Zugriff auf Prognosen erforderlich.", Vi: "Cần quyền truy cập dự báo để xem biến động đã ghi nhận."},
	MovementScope:        langcopy.Phrase{En: "Choose a fixed population and fiscal-quarter close window for movement", De: "Wähle für die Veränderungen einen festen Personenkreis und ein Abschlussfenster innerhalb eines Geschäftsquartals aus.", Vi: "Chọn nhóm cố định và khoảng chốt theo quý tài chính để xem biến động."},
	RestrictedMovement:   langcopy.Phrase{En: "Some captured contributions are restricted", De: "Einige erfasste Beiträge sind zugriffsbeschränkt.", Vi: "Một số khoản đóng góp đã ghi nhận bị hạn chế truy cập."},
	SingleTeam:           langcopy.Phrase{En: "Choose a single team for its forecast", De: "Wähle ein einzelnes Team aus, um seine Prognose zu sehen.", Vi: "Chọn một nhóm để xem dự báo của nhóm đó."},
	CalculationFailed:    langcopy.Phrase{En: "This figure could not be calculated just now. The other figures are not affected; reload to try again.", De: "Diese Zahl konnte gerade nicht berechnet werden. Die anderen Zahlen sind nicht betroffen. Lade die Seite neu, um es noch einmal zu versuchen.", Vi: "Không thể tính số liệu này lúc này. Các số liệu khác không bị ảnh hưởng; hãy tải lại để thử lại."},
	NoPipeline:           langcopy.Phrase{En: "This figure is not split by pipeline; choose all pipelines", De: "Diese Zahl wird nicht nach Pipeline aufgeteilt. Wähle alle Pipelines aus.", Vi: "Số liệu này không chia theo pipeline; hãy chọn tất cả pipeline."},
	IndividualOff:        langcopy.Phrase{En: "An administrator switched off individual performance, so this shows your own figures only. A figure is withheld when fewer than two other members make it up, or when taking another team's total from it would leave fewer than two.", De: "Ein Admin hat die Einzelleistung ausgeschaltet. Hier stehen deshalb nur deine eigenen Zahlen. Eine Zahl wird zurückgehalten, wenn weniger als zwei andere Mitglieder zu ihr beitragen oder wenn nach Abzug der Summe eines anderen Teams weniger als zwei übrig blieben.", Vi: "Quản trị viên đã tắt hiệu suất của từng thành viên, nên ở đây chỉ hiển thị số liệu của bạn. Một số liệu bị ẩn khi có ít hơn hai thành viên khác đóng góp, hoặc khi trừ đi tổng của một nhóm khác sẽ còn ít hơn hai thành viên."},
	BelowFloor:           langcopy.Phrase{En: "Individual performance is off. A figure is withheld when fewer than two other members make it up, or when taking another team's total from it would leave fewer than two.", De: "Die Einzelleistung ist ausgeschaltet. Eine Zahl wird zurückgehalten, wenn weniger als zwei andere Mitglieder zu ihr beitragen oder wenn nach Abzug der Summe eines anderen Teams weniger als zwei übrig blieben.", Vi: "Hiệu suất của từng thành viên đang tắt. Một số liệu bị ẩn khi có ít hơn hai thành viên khác đóng góp, hoặc khi trừ đi tổng của một nhóm khác sẽ còn ít hơn hai thành viên."},
	Dimension:            langcopy.Phrase{En: "This figure is not split by product, country or industry; clear those filters to read it", De: "Diese Zahl wird nicht nach Produkt, Land oder Branche aufgeteilt. Entferne diese Filter, um sie zu lesen.", Vi: "Số liệu này không chia theo sản phẩm, quốc gia hay ngành; hãy bỏ các bộ lọc đó để đọc."},
	ChooseInitiative:     langcopy.Phrase{En: "Choose a sales initiative to read its pipeline by origin", De: "Wähle eine Vertriebsinitiative, um ihre Pipeline nach Herkunft zu lesen.", Vi: "Chọn một sáng kiến bán hàng để đọc pipeline theo nguồn gốc."},
	ValueBasisOpen:       langcopy.Phrase{En: "Open value has no software or implementation split; choose all value to read it", De: "Offener Wert wird nicht in Software und Umsetzung aufgeteilt. Wähle den gesamten Wert, um ihn zu lesen.", Vi: "Giá trị đang mở không chia thành phần mềm và triển khai; hãy chọn toàn bộ giá trị để đọc."},
}

// ComponentKindLabels name the groups of the commercial mix: each
// product.component_kind, a product nobody classified, and the part of a won
// deal no product carries.
var ComponentKindLabels = map[string]langcopy.Phrase{
	string(crmcontracts.DealLineComponentKindSoftware):       {En: "Software", De: "Software", Vi: "Phần mềm"},
	string(crmcontracts.DealLineComponentKindImplementation): {En: "Implementation", De: "Umsetzung", Vi: "Triển khai"},
	string(crmcontracts.DealLineComponentKindOther):          {En: "Other", De: "Sonstiges", Vi: "Khác"},
	"unclassified":            {En: "Unclassified product", De: "Nicht eingeordnetes Produkt", Vi: "Sản phẩm chưa phân loại"},
	"unallocated":             {En: "Unallocated", De: "Nicht zugeordnet", Vi: "Chưa phân bổ"},
	salesprogress.InvalidKind: {En: "Lines exceed the deal value", De: "Positionen übersteigen den Dealwert", Vi: "Các dòng vượt quá giá trị deal"},
}

// ForecastWords explain a forecast figure that is only partly priced.
var ForecastWords = struct{ Unpriced, MovementUnpriced langcopy.Phrase }{
	Unpriced:         langcopy.Phrase{En: "Some forecast contributions are unpriced", De: "Für einige Prognosebeiträge fehlt die Bewertung.", Vi: "Một số khoản đóng góp vào dự báo chưa được định giá."},
	MovementUnpriced: langcopy.Phrase{En: "Some captured deals could not be valued; movement covers priced contributions only", De: "Einige erfasste Deals konnten nicht bewertet werden. Die Veränderungen enthalten nur bewertete Beiträge.", Vi: "Một số deal đã ghi nhận chưa thể định giá; biến động chỉ bao gồm các khoản đóng góp đã có giá."},
}

// ForecastCaveats explain why an expected landing shows commit evidence or a
// call below what was won.
var ForecastCaveats = map[forecasting.LandingCaveat]langcopy.Phrase{
	forecasting.CaveatCallAbsent:      {En: "No call recorded for this period, so this shows commit evidence.", De: "Angezeigt werden die Commit-Belege, da für diesen Zeitraum keine Einschätzung erfasst ist.", Vi: "Kỳ này chưa có nhận định nào, nên số này hiển thị bằng chứng Commit."},
	forecasting.CaveatCallWithheld:    {En: "The manager's call is not shown here, so this shows commit evidence.", De: "Angezeigt werden die Commit-Belege, da die Einschätzung hier nicht sichtbar ist.", Vi: "Nhận định của quản lý không hiển thị ở đây, nên số này hiển thị bằng chứng Commit."},
	forecasting.CaveatCallBelowActual: {En: "The call is below the amount already won. It is shown as recorded, not corrected.", De: "Die Einschätzung liegt unter dem bereits gewonnenen Betrag. Die Einschätzung wird wie erfasst angezeigt, nicht korrigiert.", Vi: "Nhận định thấp hơn số tiền đã chốt. Số này hiển thị đúng như đã ghi, không chỉnh sửa."},
}

// CohortWords explain what a cohort figure counts and leaves out.
var CohortWords = struct{ Arrivals, ExitNotRecorded, ArrivalOwnerUnknown langcopy.Phrase }{
	Arrivals:            langcopy.Phrase{En: "Leads that arrived", De: "Eingegangene Leads", Vi: "Lead đã đến"},
	ExitNotRecorded:     langcopy.Phrase{En: "Some deals left a stage by a move an import brought in or another CRM synced in, which does not show how your team sells, or by a move saved before Margince kept the stage order. These deals are counted apart and left out of the rate.", De: "Einige Deals haben eine Phase durch einen Wechsel verlassen, den ein Import mitgebracht oder ein anderes CRM synchronisiert hat und der nicht zeigt, wie dein Team verkauft, oder durch einen Wechsel, der gespeichert wurde, bevor Margince die Reihenfolge der Phasen festgehalten hat. Diese Deals werden getrennt gezählt und fließen nicht in die Quote ein.", Vi: "Một số deal rời giai đoạn bằng một lần chuyển do lần nhập dữ liệu mang vào hoặc được đồng bộ từ CRM khác, vốn không cho thấy cách nhóm của bạn bán hàng, hoặc bằng một lần chuyển được lưu trước khi Margince ghi thứ tự giai đoạn. Các deal này được đếm riêng và không tính vào tỷ lệ."},
	ArrivalOwnerUnknown: langcopy.Phrase{En: "Some leads arrived before Margince kept who owned a lead when it arrived. They count for the whole workspace, but for no team member or team.", De: "Einige Leads sind eingegangen, bevor Margince gespeichert hat, wem ein Lead bei seinem Eingang gehörte. Sie zählen für den gesamten Workspace, aber für kein Teammitglied und kein Team.", Vi: "Một số lead đến trước khi Margince lưu người phụ trách lead tại thời điểm lead đến. Các lead này được tính cho toàn bộ không gian làm việc, nhưng không tính cho người hay nhóm nào."},
}
