// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// withProjectColumns adds the project and product columns to the guide the
// other objects declare, and answers it. They sit apart to keep each file under
// the length cap.
func withProjectColumns(columns map[string]Column) map[string]Column {
	maps.Copy(columns, projectColumns)
	return columns
}

var projectColumns = map[string]Column{
	"project.name": {Description: langcopy.Phrase{
		En: "The project's name.",
		De: "Der Name des Projekts.",
		Vi: "Tên của dự án.",
	}, Example: langcopy.Phrase{En: "Plant rollout 2027", De: "Werksausbau 2027", Vi: "Triển khai nhà máy 2027"}, RequiredOnNew: true},
	"project.description": {Description: langcopy.Phrase{
		En: "A short description of the project.",
		De: "Eine kurze Beschreibung des Projekts.",
		Vi: "Mô tả ngắn về dự án.",
	}, Example: langcopy.Phrase{En: "New line for the second plant", De: "Neue Linie für das zweite Werk", Vi: "Dây chuyền mới cho nhà máy thứ hai"}},
	"phase": {Description: langcopy.Phrase{
		En: "The phase a new project starts in: initiative, pursuing or delivering. A project is not created closed, and a later file does not move its phase.",
		De: "Die Phase, in der ein neues Projekt startet: initiative, pursuing oder delivering. Ein Projekt wird nicht geschlossen erstellt, und eine spätere Datei ändert seine Phase nicht.",
		Vi: "Giai đoạn dự án mới bắt đầu: initiative, pursuing hoặc delivering. Dự án không được tạo ở trạng thái đã đóng, và tệp sau không đổi giai đoạn của nó.",
	}, Example: same("initiative · pursuing · delivering")},
	"project.company_ref": {Description: langcopy.Phrase{
		En: "The company the project belongs to: the company's ref in this upload or an earlier one, its domain, its exact name, or its Margince ID. A new project needs one, and a later file cannot give it another company.",
		De: "Das Unternehmen, zu dem das Projekt gehört: die Ref des Unternehmens aus diesem oder einem früheren Import, seine Domain, sein genauer Name oder seine Margince-ID. Ein neues Projekt braucht eines, und eine spätere Datei kann ihm kein anderes Unternehmen geben.",
		Vi: "Công ty mà dự án thuộc về: ref của công ty trong lần tải lên này hoặc lần trước, tên miền, tên chính xác, hoặc ID Margince. Dự án mới cần có một công ty, và tệp sau không thể đổi sang công ty khác.",
	}, Example: same("CO-0042 · acme.com"), RequiredOnNew: true},
	"deal_ref": {Description: langcopy.Phrase{
		En: "The open deal this project comes from: the deal's ref in this upload or an earlier one, or its Margince ID. The deal must belong to the same company and name no project yet. Written only when a row creates the project.",
		De: "Der offene Deal, aus dem das Projekt entsteht: die Ref des Deals aus diesem oder einem früheren Import oder seine Margince-ID. Der Deal muss zum selben Unternehmen gehören und noch kein Projekt haben. Wird nur geschrieben, wenn eine Zeile das Projekt erstellt.",
		Vi: "Deal đang mở mà dự án bắt nguồn từ đó: ref của deal trong lần tải lên này hoặc lần trước, hoặc ID Margince. Deal phải thuộc cùng công ty và chưa có dự án. Chỉ được ghi khi dòng tạo dự án mới.",
	}, Example: same("D-1001")},
	"started_at": {Description: langcopy.Phrase{
		En: "The day the project started, as YYYY-MM-DD.",
		De: "Der Tag, an dem das Projekt startete, im Format YYYY-MM-DD.",
		Vi: "Ngày dự án bắt đầu, theo dạng YYYY-MM-DD.",
	}, Example: same("2026-09-01")},
	"target_end_date": {Description: langcopy.Phrase{
		En: "The day the project should end, as YYYY-MM-DD. It cannot be before the start date.",
		De: "Der Tag, an dem das Projekt enden soll, im Format YYYY-MM-DD. Er darf nicht vor dem Startdatum liegen.",
		Vi: "Ngày dự án dự kiến kết thúc, theo dạng YYYY-MM-DD. Không được trước ngày bắt đầu.",
	}, Example: same("2027-03-31")},
	"product.name": {Description: langcopy.Phrase{
		En: "The product's name, as an offer line shows it.",
		De: "Der Name des Produkts, so wie eine Angebotsposition ihn zeigt.",
		Vi: "Tên sản phẩm, như dòng báo giá hiển thị.",
	}, Example: langcopy.Phrase{En: "Annual support", De: "Jahressupport", Vi: "Hỗ trợ hằng năm"}, RequiredOnNew: true},
	"product.description": {Description: langcopy.Phrase{
		En: "A short description of the product.",
		De: "Eine kurze Beschreibung des Produkts.",
		Vi: "Mô tả ngắn về sản phẩm.",
	}, Example: langcopy.Phrase{En: "Support with a four-hour reply", De: "Support mit vier Stunden Antwortzeit", Vi: "Hỗ trợ phản hồi trong bốn giờ"}},
	"sku": {Description: langcopy.Phrase{
		En: "The product's article number. No other live product may have it. Upload again with the same SKU, and Margince updates the product instead of making a second one.",
		De: "Die Artikelnummer des Produkts. Kein anderes aktives Produkt darf sie tragen. Lädst du mit derselben Artikelnummer erneut hoch, aktualisiert Margince das Produkt, statt ein zweites zu erstellen.",
		Vi: "Mã hàng của sản phẩm. Không sản phẩm đang dùng nào khác được có mã này. Tải lên lại với cùng mã, Margince sẽ cập nhật sản phẩm thay vì tạo sản phẩm thứ hai.",
	}, Example: same("SUP-1Y")},
	"unit": {Description: langcopy.Phrase{
		En: "What one price covers, such as item, hour, day or month. Any text you choose.",
		De: "Wofür ein Preis gilt, etwa Stück, Stunde, Tag oder Monat. Ein beliebiger Text.",
		Vi: "Một mức giá tính cho gì, chẳng hạn cái, giờ, ngày hoặc tháng. Văn bản bất kỳ.",
	}, Example: langcopy.Phrase{En: "year", De: "Jahr", Vi: "năm"}},
	"unit_price": {Description: langcopy.Phrase{
		En: "The price of one unit, in the major units of the currency column, with a dot for decimals.",
		De: "Der Preis einer Einheit in der Hauptwährungseinheit der Währungsspalte, mit Punkt als Dezimaltrennzeichen.",
		Vi: "Giá của một đơn vị, tính theo đơn vị chính của cột tiền tệ, dùng dấu chấm cho phần thập phân.",
	}, Example: same("1200.00"), RequiredOnNew: true},
	"default_tax_rate": {Description: langcopy.Phrase{
		En: "The tax rate an offer line starts with, in percent.",
		De: "Der Steuersatz, mit dem eine Angebotsposition startet, in Prozent.",
		Vi: "Thuế suất mà dòng báo giá bắt đầu, tính theo phần trăm.",
	}, Example: same("19")},
	"tax_category": {Description: langcopy.Phrase{
		En: "The tax category: standard, reduced or exempt. Leave it empty if nobody has classified the product.",
		De: "Die Steuerkategorie: standard, reduced oder exempt. Lass sie leer, wenn niemand das Produkt eingeordnet hat.",
		Vi: "Nhóm thuế: standard, reduced hoặc exempt. Để trống nếu chưa ai phân loại sản phẩm.",
	}, Example: same("standard · reduced · exempt")},
	"billing_model": {Description: langcopy.Phrase{
		En: "Whether the price is paid once or repeats: one_time or recurring. A recurring price needs the billing interval, and a one-time price has none.",
		De: "Ob der Preis einmal anfällt oder sich wiederholt: one_time oder recurring. Ein wiederkehrender Preis braucht das Abrechnungsintervall, ein einmaliger hat keines.",
		Vi: "Giá trả một lần hay lặp lại: one_time hoặc recurring. Giá lặp lại cần chu kỳ thanh toán, giá một lần thì không có.",
	}, Example: same("one_time · recurring")},
	"billing_interval_months": {Description: langcopy.Phrase{
		En: "The number of months one billing period spans. Only for a recurring price.",
		De: "Die Anzahl der Monate, die ein Abrechnungszeitraum umfasst. Nur für einen wiederkehrenden Preis.",
		Vi: "Số tháng của một kỳ thanh toán. Chỉ dùng cho giá lặp lại.",
	}, Example: same("12")},
	"active": {Description: langcopy.Phrase{
		En: "Whether an offer may use the product: yes or no. A product starts active.",
		De: "Ob ein Angebot das Produkt verwenden darf: yes oder no. Ein Produkt startet aktiv.",
		Vi: "Báo giá có được dùng sản phẩm không: yes hoặc no. Sản phẩm bắt đầu ở trạng thái hoạt động.",
	}, Example: same("yes · no")},
}
