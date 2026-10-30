// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// reportingMetricNames are the names the screen and the CSV export show for
// each metric. The id stays the machine identifier.
//
//nolint:dupl // every metric table is a map of phrases, so the tables share one shape and no text
var reportingMetricNames = map[crmcontracts.ReportingMetricID]langcopy.Phrase{
	reportingBookingsWon:              {En: "Sales won", De: "Gewonnener Umsatz", Vi: "Doanh số đã chốt"},
	reportingClosedWinRate:            {En: "Closed win rate", De: "Gewinnquote abgeschlossener Deals", Vi: "Tỷ lệ thắng trên deal đã đóng"},
	reportingOpenPipeline:             {En: "Open pipeline", De: "Offene Pipeline", Vi: "Pipeline đang mở"},
	reportingStageAge:                 {En: "Time in current stage", De: "Zeit in der aktuellen Phase", Vi: "Thời gian ở giai đoạn hiện tại"},
	reportingQualifiedPipelineCreated: {En: "Qualified pipeline created", De: "Neu qualifizierte Pipeline", Vi: "Pipeline đủ điều kiện đã tạo"},
	reportingMeetingsHeld:             {En: "Meetings held", De: "Durchgeführte Termine", Vi: "Cuộc họp đã diễn ra"},
	reportingAcceptedOpportunities:    {En: "Accepted handoffs", De: "Angenommene Übergaben", Vi: "Bàn giao đã chấp nhận"},
	reportingForecastLanding:          {En: "Forecast landing", De: "Erwartetes Ergebnis", Vi: "Dự kiến cuối kỳ"},
	reportingLeadsCreated:             {En: "New leads", De: "Neue Leads", Vi: "Lead mới"},
	reportingLeadStatusMoves:          {En: "Lead status changes", De: "Statuswechsel von Leads", Vi: "Lượt đổi trạng thái lead"},
	reportingLeadsPromoted:            {En: "Leads qualified", De: "Qualifizierte Leads", Vi: "Lead đủ điều kiện"},
	reportingLeadFirstResponse:        {En: "First response time", De: "Zeit bis zur ersten Antwort", Vi: "Thời gian phản hồi đầu tiên"},
	reportingLeadsAwaitingResponse:    {En: "Leads still waiting for a reply", De: "Leads, die noch auf eine Antwort warten", Vi: "Lead vẫn đang chờ phản hồi"},
	reportingPartnerPipeline:          {En: "Partner pipeline", De: "Pipeline über Partnerunternehmen", Vi: "Pipeline từ đối tác"},
	reportingPartnerWon:               {En: "Partner sales won", De: "Umsatz über Partnerunternehmen", Vi: "Doanh số chốt từ đối tác"},
	reportingPartnerCommission:        {En: "Partner commission", De: "Provision für Partnerunternehmen", Vi: "Hoa hồng đối tác"},
	reportingPartnerActivity:          {En: "Active partners", De: "Aktive Partnerunternehmen", Vi: "Đối tác đang hoạt động"},
}

// reportingMetricName names a metric in the reading's language. An edition
// keeps no name, so a metric retired since it was captured says so.
func reportingMetricName(frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID) string {
	name, ok := reportingMetricNames[metric]
	if !ok {
		name = reportingLabelWords.MetricRetired
	}
	return reporting.ExplanationCopy(frame).Say(name)
}

// The tables answer what a salesperson asks about a figure: what it counts, who
// gets credit, what missing data does. Each sentence states only what that
// metric's reader computes.
var reportingDefinitionWords = map[crmcontracts.ReportingMetricID]langcopy.Phrase{
	reportingBookingsWon: {
		En: "The total value of deals won with a close date in the period you picked, credited to the deal's current owner. Example: a €10,000 deal marked won on 12 March counts in March, even if its expected close date was in April.",
		De: "Der Gesamtwert der gewonnenen Deals mit einem Abschlussdatum im gewählten Zeitraum, gutgeschrieben der Person, die den Deal jetzt verantwortet. Beispiel: Ein Deal über 10.000 €, gewonnen am 12. März, zählt im März, auch wenn der erwartete Abschluss im April lag.",
		Vi: "Tổng giá trị các deal đã thắng có ngày chốt trong khoảng thời gian bạn chọn, tính cho người đang phụ trách deal. Ví dụ: deal 10.000 € thắng ngày 12.03 được tính vào tháng 3, kể cả khi ngày chốt dự kiến là tháng 4.",
	},
	reportingClosedWinRate: {
		En: "Of the deals won or lost with a close date in the period you picked, the share that was won, credited to the deal's current owner. Example: 6 deals won and 4 lost in March give a win rate of 60%.",
		De: "Von allen Deals, die im gewählten Zeitraum gewonnen oder verloren wurden, der Anteil der gewonnenen, gutgeschrieben der Person, die den Deal jetzt verantwortet. Beispiel: 6 gewonnene und 4 verlorene Deals im März ergeben eine Gewinnquote von 60 %.",
		Vi: "Trong các deal đã thắng hoặc thua có ngày đóng trong khoảng thời gian bạn chọn, phần deal đã thắng, tính cho người đang phụ trách deal. Ví dụ: tháng 3 có 6 deal thắng và 4 deal thua thì tỷ lệ thắng là 60%.",
	},
	reportingOpenPipeline: {
		En: "The total value of all deals open right now, credited to the deal's current owner; the period you pick does not change it, only the expected close filter does. Example: three open deals of €5,000 each give €15,000, whichever period is chosen.",
		De: "Der Gesamtwert aller Deals, die gerade offen sind, gutgeschrieben der Person, die den Deal jetzt verantwortet; der gewählte Zeitraum ändert die Zahl nicht, nur der Filter für den erwarteten Abschluss. Beispiel: Drei offene Deals über je 5.000 € ergeben 15.000 €, egal welcher Zeitraum gewählt ist.",
		Vi: "Tổng giá trị tất cả deal đang mở ngay lúc này, tính cho người đang phụ trách deal; khoảng thời gian bạn chọn không làm thay đổi con số này, chỉ bộ lọc ngày chốt dự kiến mới có tác dụng. Ví dụ: ba deal đang mở, mỗi deal 5.000 €, cho 15.000 € dù chọn khoảng thời gian nào.",
	},
	reportingStageAge: {
		En: "How many whole days open deals have spent in the stage they are in now, counted from the last time they entered it; the figure is the middle value, so about half the deals have waited longer. Example: deals waiting 2, 3, 5, 9 and 30 days give 5 days.",
		De: "Wie viele volle Tage offene Deals schon in ihrer jetzigen Phase sind, gezählt ab dem letzten Eintritt in diese Phase; angezeigt wird der mittlere Wert, etwa die Hälfte der Deals wartet also länger. Beispiel: Deals, die seit 2, 3, 5, 9 und 30 Tagen warten, ergeben 5 Tage.",
		Vi: "Số ngày trọn vẹn các deal đang mở đã nằm ở giai đoạn hiện tại, tính từ lần gần nhất deal vào giai đoạn đó; con số là giá trị ở giữa, nên khoảng một nửa số deal đã chờ lâu hơn. Ví dụ: các deal đã chờ 2, 3, 5, 9 và 30 ngày cho kết quả 5 ngày.",
	},
	reportingQualifiedPipelineCreated: {
		En: "The total value of deals that reached a qualifying stage for the first time in the period you picked, credited to whoever owned the deal at that moment. Example: a €8,000 deal that moves into a qualifying stage on 4 May counts €8,000 in May, and moving back and forward again later adds nothing.",
		De: "Der Gesamtwert der Deals, die im gewählten Zeitraum zum ersten Mal eine qualifizierende Phase erreicht haben, gutgeschrieben der Person, die den Deal in diesem Moment verantwortet hat. Beispiel: Ein Deal über 8.000 €, der am 4. Mai in eine qualifizierende Phase wechselt, zählt 8.000 € im Mai; wechselt er später zurück und wieder vor, kommt nichts hinzu.",
		Vi: "Tổng giá trị các deal lần đầu vào một giai đoạn đủ điều kiện trong khoảng thời gian bạn chọn, tính cho người phụ trách deal tại thời điểm đó. Ví dụ: deal 8.000 € vào giai đoạn đủ điều kiện ngày 04.05 được tính 8.000 € vào tháng 5; lùi lại rồi vào lại sau đó không cộng thêm.",
	},
	reportingMeetingsHeld: {
		En: "The number of meetings with a contact, lead or company that took place in the period you picked and are marked as held, credited to the meeting's host. Example: a customer meeting on 3 June marked as held counts once in June; a meeting not yet held does not count.",
		De: "Die Zahl der Termine mit einem Kontakt, Lead oder Unternehmen, die im gewählten Zeitraum stattfanden und als durchgeführt markiert sind, gutgeschrieben der Person, die den Termin leitet. Beispiel: Ein Termin mit Kundschaft am 3. Juni, der als durchgeführt markiert ist, zählt einmal im Juni; ein Termin, der noch nicht stattgefunden hat, zählt nicht.",
		Vi: "Số cuộc họp với một liên hệ, lead hoặc công ty diễn ra trong khoảng thời gian bạn chọn và được đánh dấu là đã diễn ra, tính cho người chủ trì cuộc họp. Ví dụ: cuộc họp với khách hàng ngày 03.06 được đánh dấu đã diễn ra được tính một lần vào tháng 6; cuộc họp chưa diễn ra không được tính.",
	},
	reportingAcceptedOpportunities: {
		En: "The number of deals whose handoff was accepted for the first time in the period you picked, credited to whoever submitted the handoff. Example: a deal handed off twice and accepted on 10 April counts once, in April.",
		De: "Die Zahl der Deals, deren Übergabe im gewählten Zeitraum zum ersten Mal angenommen wurde, gutgeschrieben der Person, die die Übergabe eingereicht hat. Beispiel: Ein Deal, der zweimal übergeben und am 10. April angenommen wurde, zählt einmal, im April.",
		Vi: "Số deal có bàn giao được chấp nhận lần đầu trong khoảng thời gian bạn chọn, tính cho người đã gửi bàn giao. Ví dụ: deal được bàn giao hai lần và được chấp nhận ngày 10.04 chỉ được tính một lần, vào tháng 4.",
	},
	reportingForecastLanding: {
		En: "Where this fiscal quarter is expected to finish, meaning the deals already won this quarter plus the open deals the projection basis adds. Example: €40,000 won so far plus €25,000 in committed deals with confirmed close dates give a landing of €65,000.",
		De: "Wo dieses Geschäftsquartal voraussichtlich endet, also die in diesem Quartal gewonnenen Deals plus die offenen Deals, die die Grundlage der Hochrechnung hinzunimmt. Beispiel: 40.000 € bereits gewonnen plus 25.000 € aus Commit-Deals mit bestätigtem Abschlussdatum ergeben ein erwartetes Ergebnis von 65.000 €.",
		Vi: "Con số quý tài chính này dự kiến đạt được, gồm các deal đã thắng trong quý cộng với các deal đang mở mà cơ sở tính dự kiến cộng vào. Ví dụ: đã thắng 40.000 € cộng 25.000 € từ các deal Commit có ngày chốt đã xác nhận cho kết quả dự kiến 65.000 €.",
	},
	reportingLeadsCreated: {
		En: "The number of leads that came in during the period you picked, counted on the day each was created and credited to the lead's current owner. Example: a lead created on 2 May counts once in May, even after it is qualified or disqualified.",
		De: "Die Zahl der Leads, die im gewählten Zeitraum eingegangen sind, gezählt am Tag ihrer Erstellung und gutgeschrieben der Person, die den Lead jetzt verantwortet. Beispiel: Ein am 2. Mai erstellter Lead zählt einmal im Mai, auch wenn er später qualifiziert oder disqualifiziert wird.",
		Vi: "Số lead đến trong khoảng thời gian bạn chọn, tính theo ngày tạo của từng lead, tính cho người đang phụ trách lead. Ví dụ: lead tạo ngày 02.05 được tính một lần vào tháng 5, kể cả khi sau đó đủ điều kiện hoặc bị loại.",
	},
	reportingLeadStatusMoves: {
		En: "The number of times leads moved to Contacted, Engaged, Qualified or Disqualified in the period you picked, counted on the day of each move and credited to the lead's current owner. Example: a lead moved from New to Contacted on 3 May and to Engaged on 9 May counts twice in May.",
		De: "Wie oft Leads im gewählten Zeitraum auf Kontaktiert, Im Gespräch, Qualifiziert oder Disqualifiziert gewechselt sind, gezählt am Tag jedes Wechsels und gutgeschrieben der Person, die den Lead jetzt verantwortet. Beispiel: Ein Lead, der am 3. Mai von Neu auf Kontaktiert und am 9. Mai auf Im Gespräch wechselt, zählt im Mai zweimal.",
		Vi: "Số lần lead chuyển sang Đã liên hệ, Đang trao đổi, Đủ điều kiện hoặc Đã loại trong khoảng thời gian bạn chọn, tính theo ngày của từng lần chuyển, tính cho người đang phụ trách lead. Ví dụ: lead chuyển từ Mới sang Đã liên hệ ngày 03.05 và sang Đang trao đổi ngày 09.05 được tính hai lần vào tháng 5.",
	},
	reportingLeadsPromoted: {
		En: "Of the leads that came in during the period you picked, the number since qualified into a deal, credited to the lead's current owner; the reading also shows how many came in. Example: 20 leads came in during May and 5 of them were qualified into a deal, so May shows 5 of 20.",
		De: "Von den Leads, die im gewählten Zeitraum eingegangen sind, die Zahl derer, die seitdem zu einem Deal qualifiziert wurden, gutgeschrieben der Person, die den Lead jetzt verantwortet; daneben steht, wie viele eingegangen sind. Beispiel: Im Mai gehen 20 Leads ein und 5 davon werden zu einem Deal qualifiziert, also zeigt der Mai 5 von 20.",
		Vi: "Trong các lead đến trong khoảng thời gian bạn chọn, số lead sau đó đủ điều kiện thành deal, tính cho người đang phụ trách lead; kèm theo là tổng số lead đã đến. Ví dụ: tháng 5 có 20 lead đến và 5 lead trong số đó thành deal, nên tháng 5 hiển thị 5 trên 20.",
	},
	reportingLeadFirstResponse: {
		En: "How many hours leads waited for a first response, from when each was routed, or created if never routed; the figure is the middle value, so about half the leads waited longer. Example: answers after 1, 2, 4, 8 and 30 hours give 4 hours.",
		De: "Wie viele Stunden Leads auf eine erste Antwort gewartet haben, ab der Zuweisung oder, ohne Zuweisung, ab der Erstellung; angezeigt wird der mittlere Wert, etwa die Hälfte der Leads hat also länger gewartet. Beispiel: Antworten nach 1, 2, 4, 8 und 30 Stunden ergeben 4 Stunden.",
		Vi: "Số giờ lead chờ phản hồi đầu tiên, tính từ lúc được phân công, hoặc lúc tạo nếu chưa phân công; con số là giá trị ở giữa, nên khoảng một nửa số lead đã chờ lâu hơn. Ví dụ: phản hồi sau 1, 2, 4, 8 và 30 giờ cho kết quả 4 giờ.",
	},
	reportingLeadsAwaitingResponse: {
		En: "Of the leads that wrote in by email during the period you picked, the number still waiting for a first reply now, credited to the lead's current owner. These are the leads the first response time cannot count yet. Example: 12 leads wrote in during May and 3 have had no reply, so May shows 3.",
		De: "Von den Leads, die im gewählten Zeitraum per E-Mail geschrieben haben, die Zahl derer, die jetzt noch auf eine erste Antwort warten, gutgeschrieben der Person, die den Lead jetzt verantwortet. Diese Leads kann die Zeit bis zur ersten Antwort noch nicht zählen. Beispiel: Im Mai schreiben 12 Leads und 3 haben noch keine Antwort, also zeigt der Mai 3.",
		Vi: "Trong các lead đã gửi email trong khoảng thời gian bạn chọn, số lead hiện vẫn đang chờ phản hồi đầu tiên, tính cho người đang phụ trách lead. Thời gian phản hồi đầu tiên chưa thể tính các lead này. Ví dụ: tháng 5 có 12 lead gửi email và 3 lead chưa được phản hồi, nên tháng 5 hiển thị 3.",
	},
	reportingPartnerPipeline: {
		En: "The total value of deals created in the period you picked that name a partner, split by partner and by whether the partner sourced or influenced the deal, credited to the deal's current owner. Example: a €20,000 deal a partner sourced, created on 6 May, adds €20,000 to that partner's sourced pipeline in May.",
		De: "Der Gesamtwert der im gewählten Zeitraum erstellten Deals, die ein Partnerunternehmen nennen, aufgeteilt nach Partnerunternehmen und danach, ob es den Deal vermittelt oder beeinflusst hat, gutgeschrieben der Person, die den Deal jetzt verantwortet. Beispiel: Ein vermittelter Deal über 20.000 €, erstellt am 6. Mai, zählt 20.000 € als vermittelt im Mai.",
		Vi: "Tổng giá trị các deal tạo trong khoảng thời gian bạn chọn có ghi một đối tác, chia theo đối tác và theo việc đối tác mang deal về hay hỗ trợ deal, tính cho người đang phụ trách deal. Ví dụ: deal 20.000 € do đối tác mang về, tạo ngày 06.05, cộng 20.000 € vào phần mang deal về của đối tác đó trong tháng 5.",
	},
	reportingPartnerWon: {
		En: "The total value of deals won in the period you picked that name a partner, split by partner and by whether the partner sourced or influenced the deal, credited to the deal's current owner. Example: a €12,000 deal a partner influenced, won on 14 May, counts €12,000 as influenced in May.",
		De: "Der Gesamtwert der im gewählten Zeitraum gewonnenen Deals, die ein Partnerunternehmen nennen, aufgeteilt nach Partnerunternehmen und danach, ob es den Deal vermittelt oder beeinflusst hat, gutgeschrieben der Person, die den Deal jetzt verantwortet. Beispiel: Ein beeinflusster Deal über 12.000 €, gewonnen am 14. Mai, zählt 12.000 € als beeinflusst im Mai.",
		Vi: "Tổng giá trị các deal thắng trong khoảng thời gian bạn chọn có ghi một đối tác, chia theo đối tác và theo việc đối tác mang deal về hay hỗ trợ deal, tính cho người đang phụ trách deal. Ví dụ: deal 12.000 € do đối tác hỗ trợ, thắng ngày 14.05, được tính 12.000 € ở phần hỗ trợ trong tháng 5.",
	},
	reportingPartnerCommission: {
		En: "The total commission partners earned in the period you picked, read from the commission ledger by the day each entry was recorded, split by partner and credited to the deal's current owner. Example: a commission of €2,000 on a deal won on 2 June counts €2,000 in June.",
		De: "Die Provision, die Partnerunternehmen im gewählten Zeitraum verdient haben, gelesen aus dem Provisionsbuch nach dem Tag, an dem jeder Eintrag erfasst wurde, aufgeteilt nach Partnerunternehmen und gutgeschrieben der Person, die den Deal jetzt verantwortet. Beispiel: Eine Provision von 2.000 € auf einen am 2. Juni gewonnenen Deal zählt 2.000 € im Juni.",
		Vi: "Tổng hoa hồng đối tác kiếm được trong khoảng thời gian bạn chọn, đọc từ sổ hoa hồng theo ngày ghi nhận từng mục, chia theo đối tác và tính cho người đang phụ trách deal. Ví dụ: hoa hồng 2.000 € trên deal thắng ngày 02.06 được tính 2.000 € vào tháng 6.",
	},
	reportingPartnerActivity: {
		En: "The number of active partners right now, out of all partners not archived: a partner is active when it is certified and an activity was logged on its company within the past 90 days, credited to the owner of the partner's company. Example: 12 partners, of which 8 are certified and had an activity in the last 90 days, show 8 of 12.",
		De: "Die Zahl der aktiven Partnerunternehmen gerade jetzt, von allen nicht archivierten: aktiv ist ein Partnerunternehmen, wenn es zertifiziert ist und in den letzten 90 Tagen eine Aktivität beim Unternehmen erfasst wurde, gutgeschrieben der Person, die das Unternehmen verantwortet. Beispiel: Von 12 Partnerunternehmen sind 8 zertifiziert und hatten in den letzten 90 Tagen eine Aktivität, also zeigt die Zahl 8 von 12.",
		Vi: "Số đối tác đang hoạt động ngay lúc này, trên tổng số đối tác chưa lưu trữ: đối tác đang hoạt động khi đã được chứng nhận và công ty của đối tác có hoạt động được ghi nhận trong 90 ngày qua, tính cho người phụ trách công ty của đối tác. Ví dụ: có 12 đối tác, 8 đối tác đã chứng nhận và có hoạt động trong 90 ngày qua, nên hiển thị 8 trên 12.",
	},
}

//nolint:dupl // every metric table is a map of phrases, so the three share one shape and no text
var reportingAttributionWords = map[crmcontracts.ReportingMetricID]langcopy.Phrase{
	reportingBookingsWon: {
		En: "Credit goes to the deal's current owner and pipeline, so a reassigned deal moves with it; a saved snapshot keeps the owner it had when it was captured.",
		De: "Gutgeschrieben wird der aktuell verantwortlichen Person und der aktuellen Pipeline des Deals, ein neu zugewiesener Deal wandert also mit; ein gespeicherter Snapshot behält die Zuordnung vom Zeitpunkt der Erfassung.",
		Vi: "Kết quả tính cho người đang phụ trách và pipeline hiện tại của deal, nên deal được chuyển giao sẽ đi theo người mới; ảnh chụp đã lưu giữ nguyên người phụ trách tại lúc ghi nhận.",
	},
	reportingClosedWinRate: {
		En: "Credit goes to the deal's current owner and pipeline; a reopened deal counts again only once it is won or lost anew.",
		De: "Gutgeschrieben wird der aktuell verantwortlichen Person und der aktuellen Pipeline des Deals; ein wiedereröffneter Deal zählt erst wieder, wenn er erneut gewonnen oder verloren ist.",
		Vi: "Kết quả tính cho người đang phụ trách và pipeline hiện tại của deal; deal được mở lại chỉ được tính lại khi đã thắng hoặc thua lần nữa.",
	},
	reportingOpenPipeline: {
		En: "Credit goes to the deal's current owner and pipeline, as they are at the moment the figure is read.",
		De: "Gutgeschrieben wird der aktuell verantwortlichen Person und der aktuellen Pipeline des Deals, so wie sie beim Abruf der Zahl sind.",
		Vi: "Kết quả tính cho người đang phụ trách và pipeline hiện tại của deal, tại thời điểm số liệu được đọc.",
	},
	reportingStageAge: {
		En: "Each deal counts for its current owner and pipeline, measured at the moment the figure is read.",
		De: "Jeder Deal zählt für die aktuell verantwortliche Person und seine aktuelle Pipeline, gemessen beim Abruf der Zahl.",
		Vi: "Mỗi deal được tính cho người đang phụ trách và pipeline hiện tại, đo tại thời điểm số liệu được đọc.",
	},
	reportingQualifiedPipelineCreated: {
		En: "Owner, pipeline and value are fixed at the first move into a stage chosen as qualifying in Reporting setup; a move that was undone does not count.",
		De: "Zuordnung, Pipeline und Wert werden beim ersten Wechsel in eine Phase festgehalten, die in den Berichtseinstellungen als qualifizierend gewählt ist; ein rückgängig gemachter Wechsel zählt nicht.",
		Vi: "Người phụ trách, pipeline và giá trị được giữ cố định tại lần đầu vào giai đoạn được chọn là đủ điều kiện trong Thiết lập báo cáo; lần chuyển đã bị hoàn tác không được tính.",
	},
	reportingMeetingsHeld: {
		En: "Credit goes to the host recorded when the meeting was marked held; older meetings without that record count for their current host.",
		De: "Gutgeschrieben wird die leitende Person, die beim Markieren als durchgeführt erfasst wurde; ältere Termine ohne diese Angabe zählen für die aktuell leitende Person.",
		Vi: "Kết quả tính cho người chủ trì được ghi nhận lúc cuộc họp được đánh dấu đã diễn ra; cuộc họp cũ không có ghi nhận này tính cho người chủ trì hiện tại.",
	},
	reportingAcceptedOpportunities: {
		En: "Credit stays with whoever submitted the first accepted handoff, even after the deal is reassigned.",
		De: "Die Gutschrift bleibt bei der Person, die die erste angenommene Übergabe eingereicht hat, auch wenn der Deal später neu zugewiesen wird.",
		Vi: "Kết quả vẫn thuộc người đã gửi bàn giao được chấp nhận đầu tiên, kể cả khi deal được chuyển cho người phụ trách khác.",
	},
	reportingForecastLanding: {
		En: "Each deal counts for its current owner; when the projection basis is the manager's call, that number replaces the whole total and is not split across owners.",
		De: "Jeder Deal zählt für die aktuell verantwortliche Person; ist die Einschätzung der Führungskraft die Grundlage der Hochrechnung, ersetzt diese Zahl die gesamte Summe und wird nicht auf einzelne Verantwortliche verteilt.",
		Vi: "Mỗi deal được tính cho người đang phụ trách; khi cơ sở tính dự kiến là nhận định của quản lý, con số đó thay cho toàn bộ tổng và không chia cho từng người.",
	},
	reportingLeadsCreated: {
		En: "Credit goes to the lead's current owner, and the breakdown splits leads by their origin.",
		De: "Gutgeschrieben wird der Person, die den Lead jetzt verantwortet, und die Aufteilung trennt Leads nach ihrer Herkunft.",
		Vi: "Kết quả tính cho người đang phụ trách lead, và phần chia nhỏ tách lead theo nguồn gốc.",
	},
	reportingLeadStatusMoves: {
		En: "Credit goes to the lead's current owner; a step back, such as Engaged to Contacted, does not count, the same rule the weekly summary counts by.",
		De: "Gutgeschrieben wird der Person, die den Lead jetzt verantwortet; ein Schritt zurück, etwa von Im Gespräch auf Kontaktiert, zählt nicht, nach derselben Regel wie in der Wochenzusammenfassung.",
		Vi: "Kết quả tính cho người đang phụ trách lead; lùi lại, như từ Đang trao đổi về Đã liên hệ, không được tính, theo cùng quy tắc với bản tóm tắt hằng tuần.",
	},
	reportingLeadsPromoted: {
		En: "Each lead counts in the period it came in, however late it is qualified, and only a qualification that names a deal counts.",
		De: "Jeder Lead zählt im Zeitraum seines Eingangs, egal wie spät er qualifiziert wird, und nur eine Qualifizierung, die einen Deal nennt, zählt.",
		Vi: "Mỗi lead được tính vào kỳ lead đến, dù đủ điều kiện muộn đến đâu, và chỉ tính lần đánh giá đủ điều kiện có gắn với một deal.",
	},
	reportingLeadFirstResponse: {
		En: "A lead counts in the period it arrived, for its current owner.",
		De: "Ein Lead zählt im Zeitraum seines Eingangs für die Person, die ihn jetzt verantwortet.",
		Vi: "Mỗi lead được tính vào kỳ lead đến, cho người đang phụ trách lead.",
	},
	reportingLeadsAwaitingResponse: {
		En: "A lead counts in the period it arrived, for its current owner.",
		De: "Ein Lead zählt im Zeitraum seines Eingangs für die Person, die ihn jetzt verantwortet.",
		Vi: "Mỗi lead được tính vào kỳ lead đến, cho người đang phụ trách lead.",
	},
	reportingPartnerPipeline: {
		En: "The partner and its role are read from the deal as it is now; a deal with no role set counts as sourced, as the deal page says.",
		De: "Partnerunternehmen und Rolle werden aus dem Deal gelesen, wie er jetzt ist; ein Deal ohne festgelegte Rolle zählt als vermittelt, wie auf der Deal-Seite.",
		Vi: "Đối tác và vai trò được đọc từ deal như hiện tại; deal chưa đặt vai trò được tính là mang deal về, như trên trang deal.",
	},
	reportingPartnerWon: {
		En: "Credit goes to the deal's current owner; the partner and its role are read from the deal as it is now, and a deal with no role set counts as sourced.",
		De: "Gutgeschrieben wird der Person, die den Deal jetzt verantwortet; Partnerunternehmen und Rolle stammen aus dem Deal, wie er jetzt ist, und ein Deal ohne festgelegte Rolle zählt als vermittelt.",
		Vi: "Kết quả tính cho người đang phụ trách deal; đối tác và vai trò được đọc từ deal như hiện tại, và deal chưa đặt vai trò được tính là mang deal về.",
	},
	reportingPartnerCommission: {
		En: "Each entry counts on the day it was recorded, for the partner it names and the current owner of its deal; an entry that was taken back does not count.",
		De: "Jeder Eintrag zählt am Tag seiner Erfassung für das genannte Partnerunternehmen und die Person, die seinen Deal jetzt verantwortet; ein zurückgenommener Eintrag zählt nicht.",
		Vi: "Mỗi mục được tính vào ngày ghi nhận, cho đối tác được ghi và người đang phụ trách deal; mục đã bị thu hồi không được tính.",
	},
	reportingPartnerActivity: {
		En: "Each partner counts once, for the current owner of its company, as it stands at the moment the figure is read.",
		De: "Jedes Partnerunternehmen zählt einmal für die Person, die das Unternehmen jetzt verantwortet, so wie es beim Abruf der Zahl steht.",
		Vi: "Mỗi đối tác được tính một lần cho người đang phụ trách công ty, theo trạng thái tại thời điểm số liệu được đọc.",
	},
}

//nolint:dupl // every metric table is a map of phrases, so the three share one shape and no text
var reportingIncompleteWords = map[crmcontracts.ReportingMetricID]langcopy.Phrase{
	reportingBookingsWon: {
		En: "Only deals you may see are counted; a won deal with no amount, or with no exchange rate into the reporting currency, adds nothing and marks the figure as partial.",
		De: "Gezählt werden nur Deals, die du sehen darfst; ein gewonnener Deal ohne Betrag oder ohne Wechselkurs in die Berichtswährung fließt nicht in den Wert ein, und die Zahl gilt dann als unvollständig.",
		Vi: "Chỉ tính các deal bạn được phép xem; deal đã thắng không có giá trị hoặc không có tỷ giá sang đồng tiền báo cáo sẽ không được cộng vào và số liệu được đánh dấu là bao phủ một phần.",
	},
	reportingClosedWinRate: {
		En: "No rate is shown when fewer than 5 deals closed in the period, or when details of some of them are hidden from you.",
		De: "Es wird keine Quote angezeigt, wenn im Zeitraum weniger als 5 Deals abgeschlossen wurden oder du Angaben zu einigen davon nicht sehen darfst.",
		Vi: "Không hiển thị tỷ lệ khi có ít hơn 5 deal đã đóng trong kỳ, hoặc khi bạn không được xem một số thông tin của các deal đó.",
	},
	reportingOpenPipeline: {
		En: "Only deals you may see are counted; an open deal with no amount, or with no exchange rate into the reporting currency, adds nothing and marks the figure as partial.",
		De: "Gezählt werden nur Deals, die du sehen darfst; ein offener Deal ohne Betrag oder ohne Wechselkurs in die Berichtswährung fließt nicht in den Wert ein, und die Zahl gilt dann als unvollständig.",
		Vi: "Chỉ tính các deal bạn được phép xem; deal đang mở không có giá trị hoặc không có tỷ giá sang đồng tiền báo cáo sẽ không được cộng vào và số liệu được đánh dấu là bao phủ một phần.",
	},
	reportingStageAge: {
		En: "A deal with no record of entering its stage is counted from the day it was created, and no figure is shown with fewer than 5 open deals.",
		De: "Ein Deal ohne erfassten Eintritt in seine Phase zählt ab seinem Erstellungsdatum, und bei weniger als 5 offenen Deals wird keine Zahl angezeigt.",
		Vi: "Deal không có lịch sử vào giai đoạn được tính từ ngày tạo, và không hiển thị số liệu khi có ít hơn 5 deal đang mở.",
	},
	reportingQualifiedPipelineCreated: {
		En: "There is no figure until qualifying stages are chosen in Reporting setup, and moves made before that, or without a recorded owner, mark the figure as partial.",
		De: "Ohne qualifizierende Phasen in den Berichtseinstellungen gibt es keine Zahl, und Wechsel aus der Zeit davor oder ohne erfasste verantwortliche Person machen die Zahl unvollständig.",
		Vi: "Không có số liệu cho đến khi chọn giai đoạn đủ điều kiện trong Thiết lập báo cáo; các lần chuyển trước thời điểm đó hoặc thiếu người phụ trách làm số liệu được đánh dấu là bao phủ một phần.",
	},
	reportingMeetingsHeld: {
		En: "Only meetings you may see are counted; older meetings counted for their current host mark the figure as partial, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Termine, die du sehen darfst; ältere Termine, die für die aktuell leitende Person zählen, machen die Zahl unvollständig, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các cuộc họp bạn được phép xem; cuộc họp cũ tính cho người chủ trì hiện tại làm số liệu được đánh dấu là bao phủ một phần, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingAcceptedOpportunities: {
		En: "Older handoffs with no recorded submitter or deal mark the figure as partial, and with a pipeline chosen there is no figure, because handoffs belong to no pipeline.",
		De: "Ältere Übergaben ohne erfasste einreichende Person oder ohne Deal machen die Zahl unvollständig, und mit einer gewählten Pipeline gibt es keine Zahl, weil Übergaben zu keiner Pipeline gehören.",
		Vi: "Bàn giao cũ không ghi nhận người gửi hoặc deal làm số liệu được đánh dấu là bao phủ một phần, và khi chọn một pipeline thì không có số liệu vì bàn giao không thuộc pipeline nào.",
	},
	reportingForecastLanding: {
		En: "A deal with no amount or exchange rate adds nothing; if the projection basis is the manager's call, a missing call or a chosen pipeline means the committed deals are used instead, and either way the figure is marked partial.",
		De: "Ein Deal ohne Betrag oder Wechselkurs fließt nicht ein; ist die Einschätzung der Führungskraft die Grundlage der Hochrechnung, zählen bei fehlender Einschätzung oder gewählter Pipeline stattdessen die Commit-Deals, und in beiden Fällen gilt die Zahl als unvollständig.",
		Vi: "Deal không có giá trị hoặc tỷ giá không được cộng vào; nếu cơ sở tính dự kiến là nhận định của quản lý, khi chưa có nhận định hoặc đã chọn một pipeline thì dùng các deal Commit thay thế, và trong cả hai trường hợp số liệu được đánh dấu là bao phủ một phần.",
	},
	reportingLeadsCreated: {
		En: "Only leads you may see are counted; merged duplicates and leads archived without being qualified or disqualified do not count, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Leads, die du sehen darfst; zusammengeführte Duplikate und Leads, die ohne Qualifizierung oder Disqualifizierung archiviert wurden, zählen nicht, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các lead bạn được phép xem; lead trùng đã gộp và lead đã lưu trữ mà không đủ điều kiện hay bị loại không được tính, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingLeadStatusMoves: {
		En: "Only leads you may see are counted, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Leads, die du sehen darfst, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các lead bạn được phép xem, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingLeadsPromoted: {
		En: "Only leads you may see are counted; merged duplicates and leads archived without being qualified or disqualified do not count, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Leads, die du sehen darfst; zusammengeführte Duplikate und Leads, die ohne Qualifizierung oder Disqualifizierung archiviert wurden, zählen nicht, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các lead bạn được phép xem; lead trùng đã gộp và lead đã lưu trữ mà không đủ điều kiện hay bị loại không được tính, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingLeadFirstResponse: {
		En: "Leads not answered yet, imported leads, merged duplicates and leads archived without being qualified or disqualified are left out; no figure is shown with fewer than 5 answered leads, or with a pipeline chosen.",
		De: "Noch nicht beantwortete und importierte Leads, zusammengeführte Duplikate und Leads, die ohne Qualifizierung oder Disqualifizierung archiviert wurden, bleiben außen vor; bei weniger als 5 beantworteten Leads oder mit einer gewählten Pipeline wird keine Zahl angezeigt.",
		Vi: "Lead chưa được phản hồi, lead nhập từ hệ thống khác, lead trùng đã gộp và lead đã lưu trữ mà không đủ điều kiện hay bị loại không được tính; không hiển thị số liệu khi có ít hơn 5 lead đã được phản hồi hoặc khi chọn một pipeline.",
	},
	reportingLeadsAwaitingResponse: {
		En: "Only leads you may see are counted; a lead counts only once an email from it came in, imported leads, merged duplicates and archived leads are left out, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Leads, die du sehen darfst; ein Lead zählt erst, wenn eine E-Mail von ihm eingegangen ist, importierte Leads, zusammengeführte Duplikate und archivierte Leads bleiben außen vor, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các lead bạn được phép xem; lead chỉ được tính khi đã có email từ lead gửi đến, lead nhập từ hệ thống khác, lead trùng đã gộp và lead đã lưu trữ không được tính, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingPartnerPipeline: {
		En: "Only deals you may see are counted; a deal with no amount or exchange rate adds nothing and marks the figure as partial, and partners you cannot see share one group without a name.",
		De: "Gezählt werden nur Deals, die du sehen darfst; ein Deal ohne Betrag oder Wechselkurs fließt nicht ein und die Zahl gilt dann als unvollständig, und Partnerunternehmen, die du nicht sehen darfst, stehen ohne Namen in einer gemeinsamen Gruppe.",
		Vi: "Chỉ tính các deal bạn được phép xem; deal không có giá trị hoặc tỷ giá không được cộng vào và số liệu được đánh dấu là bao phủ một phần, và các đối tác bạn không xem được nằm chung một nhóm không có tên.",
	},
	reportingPartnerWon: {
		En: "Only deals you may see are counted; a deal with no amount or exchange rate adds nothing and marks the figure as partial, and partners you cannot see share one group without a name.",
		De: "Gezählt werden nur Deals, die du sehen darfst; ein Deal ohne Betrag oder Wechselkurs fließt nicht ein und die Zahl gilt dann als unvollständig, und Partnerunternehmen, die du nicht sehen darfst, stehen ohne Namen in einer gemeinsamen Gruppe.",
		Vi: "Chỉ tính các deal bạn được phép xem; deal không có giá trị hoặc tỷ giá không được cộng vào và số liệu được đánh dấu là bao phủ một phần, và các đối tác bạn không xem được nằm chung một nhóm không có tên.",
	},
	reportingPartnerCommission: {
		En: "Only entries on deals you may see are counted; an entry in another currency with no stored exchange rate adds nothing and marks the figure as partial, partners you cannot see share one group without a name, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Einträge zu Deals, die du sehen darfst; ein Eintrag in einer anderen Währung ohne gespeicherten Wechselkurs fließt nicht ein und die Zahl gilt dann als unvollständig, Partnerunternehmen, die du nicht sehen darfst, stehen ohne Namen in einer gemeinsamen Gruppe, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các mục thuộc deal bạn được phép xem; mục bằng đồng tiền khác không có tỷ giá đã lưu sẽ không được cộng vào và số liệu được đánh dấu là bao phủ một phần, các đối tác bạn không xem được nằm chung một nhóm không có tên, và khi chọn một pipeline thì không có số liệu.",
	},
	reportingPartnerActivity: {
		En: "Only partners you may see are counted; a partner whose company has no logged activity counts as inactive, and with a pipeline chosen there is no figure.",
		De: "Gezählt werden nur Partnerunternehmen, die du sehen darfst; ohne erfasste Aktivität beim Unternehmen gilt ein Partnerunternehmen als inaktiv, und mit einer gewählten Pipeline gibt es keine Zahl.",
		Vi: "Chỉ tính các đối tác bạn được phép xem; đối tác mà công ty chưa có hoạt động nào được tính là không hoạt động, và khi chọn một pipeline thì không có số liệu.",
	},
}
