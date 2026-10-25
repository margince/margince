// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importguide holds the import format's text in every language: each
// column's guide, the key rules and the template README's own words. The
// compose import reads it; its tests hold that every target has a guide and
// every guide names a target.
package importguide

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// Column is one column's guide: what goes in it, an example, and whether a
// row that creates a record needs it.
type Column struct {
	Description, Example langcopy.Phrase
	RequiredOnNew        bool
}

// Columns is the guide of every core import target, keyed by target or by
// "object.target" where one object's column means something else.
var Columns = withActivityColumns(map[string]Column{
	"ref": {Description: langcopy.Phrase{
		En: "Your own key for this row: the ID from your old CRM, or any text you choose. Upload again with the same ref and source name, and Margince updates the record instead of making a second one. Other files use it to link to this row.",
		De: "Dein eigener Schlüssel für diese Zeile: die ID aus deinem alten CRM oder ein beliebiger Text. Lädst du mit derselben Ref und demselben Quellnamen erneut hoch, aktualisiert Margince den Datensatz, statt einen zweiten anzulegen. Andere Dateien verknüpfen über diesen Wert mit dieser Zeile.",
		Vi: "Khóa riêng của bạn cho dòng này: ID từ CRM cũ, hoặc văn bản bất kỳ bạn chọn. Tải lên lại với cùng ref và tên nguồn, Margince sẽ cập nhật hồ sơ thay vì tạo hồ sơ thứ hai. Các tệp khác dùng giá trị này để liên kết tới dòng này.",
	}, Example: same("12345 · CO-0042 · acme")},
	"id": {Description: langcopy.Phrase{
		En: "The company's Margince ID, from an export of Margince. A row with an ID updates that company and never creates one.",
		De: "Die Margince-ID des Unternehmens aus einem Export von Margince. Eine Zeile mit ID aktualisiert dieses Unternehmen und legt nie ein neues an.",
		Vi: "ID Margince của công ty, lấy từ tệp xuất của Margince. Dòng có ID sẽ cập nhật công ty đó và không bao giờ tạo công ty mới.",
	}, Example: same("0192f3a4-5b6c-7d8e-9f00-112233445566")},
	"company.display_name": {Description: langcopy.Phrase{
		En: "The company's display name. Used to find an existing company when the row has no ref.",
		De: "Der Anzeigename des Unternehmens. Hat die Zeile keine Ref, wird damit ein vorhandenes Unternehmen gesucht.",
		Vi: "Tên hiển thị của công ty. Dùng để tìm công ty đã có khi dòng không có ref.",
	}, Example: same("Acme GmbH"), RequiredOnNew: true},
	"legal_name": {Description: langcopy.Phrase{
		En: "The registered legal name, if it differs from the display name.",
		De: "Der eingetragene rechtliche Name, falls er vom Anzeigenamen abweicht.",
		Vi: "Tên pháp lý đã đăng ký, nếu khác tên hiển thị.",
	}, Example: same("Acme Industries GmbH & Co. KG")},
	"industry": {Description: langcopy.Phrase{
		En: "One of the industries Margince lists, or text that names one. Text that names none leaves Industry empty and is kept in the description.",
		De: "Eine der Branchen, die Margince führt, oder ein Text, der eine nennt. Ein Text ohne passende Branche lässt Branche leer und bleibt in der Beschreibung erhalten.",
		Vi: "Một trong các ngành Margince liệt kê, hoặc văn bản gọi tên một ngành. Văn bản không khớp ngành nào để trống ngành và được giữ trong mô tả.",
	}, Example: langcopy.Phrase{En: "Manufacturing", De: "Maschinenbau", Vi: "Sản xuất"}},
	"size_band": {Description: langcopy.Phrase{
		En: "Number of employees, as one of the bands Margince uses.",
		De: "Anzahl der Beschäftigten als eine der Größenklassen, die Margince verwendet.",
		Vi: "Số nhân viên, theo một trong các nhóm quy mô Margince dùng.",
	}, Example: same("1-10 · 11-50 · 51-200")},
	"description": {Description: langcopy.Phrase{
		En: "A short description of the company.",
		De: "Eine kurze Beschreibung des Unternehmens.",
		Vi: "Mô tả ngắn về công ty.",
	}, Example: langcopy.Phrase{En: "Maker of industrial pumps", De: "Hersteller von Industriepumpen", Vi: "Nhà sản xuất máy bơm công nghiệp"}},
	"domain": {Description: langcopy.Phrase{
		En: "The company's web domain, without https:// or www. Contacts can link to a company by its domain.",
		De: "Die Webdomain des Unternehmens, ohne https:// und www. Kontakte können über die Domain mit einem Unternehmen verknüpft werden.",
		Vi: "Tên miền web của công ty, không có https:// hay www. Liên hệ có thể liên kết tới công ty qua tên miền.",
	}, Example: same("acme.com")},
	"company.linkedin_url": {Description: langcopy.Phrase{
		En: "The full address of the company's LinkedIn page.",
		De: "Die vollständige Adresse der LinkedIn-Seite des Unternehmens.",
		Vi: "Địa chỉ đầy đủ của trang LinkedIn của công ty.",
	}, Example: same("https://www.linkedin.com/company/acme")},
	"linkedin_url": {Description: langcopy.Phrase{
		En: "The full address of the LinkedIn profile.",
		De: "Die vollständige Adresse des LinkedIn-Profils.",
		Vi: "Địa chỉ đầy đủ của hồ sơ LinkedIn cá nhân.",
	}, Example: same("https://www.linkedin.com/in/jo-tran")},
	"author": {Description: langcopy.Phrase{
		En: "Who wrote the record in the system the file comes from, as written there. When it is the email of a Margince user, the record names that user. Written only when a row creates a record.",
		De: "Wer den Datensatz im Quellsystem verfasst hat, so wie es dort steht. Ist es die E-Mail-Adresse eines Mitglieds, nennt der Datensatz dieses Mitglied. Wird nur geschrieben, wenn eine Zeile einen Datensatz anlegt.",
		Vi: "Người đã viết hồ sơ trong hệ thống nguồn, giữ nguyên như ở đó. Nếu là email của một thành viên Margince, hồ sơ sẽ ghi tên thành viên đó. Chỉ được ghi khi dòng tạo hồ sơ mới.",
	}, Example: same("mai@yourcompany.com · Mai Nguyen")},
	"owner_email": {Description: langcopy.Phrase{
		En: "Email of the Margince user who owns the record. If no user has this email, you become the owner and the import preview says so. A lead stays unassigned.",
		De: "E-Mail-Adresse des Mitglieds, das für den Datensatz verantwortlich ist. Hat kein Mitglied diese Adresse, wirst du verantwortlich, und die Importvorschau weist darauf hin. Ein Lead bleibt nicht zugewiesen.",
		Vi: "Email của thành viên Margince chịu trách nhiệm hồ sơ. Nếu không thành viên nào có email này, bạn sẽ là người chịu trách nhiệm và bản xem trước lần nhập sẽ báo điều này. Lead vẫn chưa giao.",
	}, Example: same("mai@yourcompany.com")},
	"tags": {Description: langcopy.Phrase{
		En: "Tags, separated by semicolons. Tags that do not exist yet are created, and the import preview lists them.",
		De: "Tags, durch Semikolons getrennt. Noch nicht vorhandene Tags werden angelegt, und die Importvorschau listet sie auf.",
		Vi: "Thẻ, phân cách bằng dấu chấm phẩy. Thẻ chưa có sẽ được tạo, và bản xem trước lần nhập liệt kê các thẻ này.",
	}, Example: same("Partner; DACH")},
	"address.line1": {Description: langcopy.Phrase{
		En: "Street and house number. A whole address here, with no other address column, is split into its parts.",
		De: "Straße und Hausnummer. Eine ganze Adresse hier, ohne weitere Adressspalte, wird in ihre Teile aufgeteilt.",
		Vi: "Số nhà và tên đường. Một địa chỉ đầy đủ ở đây, khi không có cột địa chỉ nào khác, được tách thành các phần.",
	}, Example: same("Hauptstraße 5")},
	"address.line2": {Description: langcopy.Phrase{
		En: "A second address line: building, floor or suite.",
		De: "Zweite Adresszeile: Gebäude, Etage oder Büro.",
		Vi: "Dòng địa chỉ thứ hai: tòa nhà, tầng hoặc phòng.",
	}, Example: langcopy.Phrase{En: "Floor 3", De: "3. Etage", Vi: "Tầng 3"}},
	"address.city": {Description: langcopy.Phrase{
		En: "City.",
		De: "Stadt.",
		Vi: "Thành phố.",
	}, Example: same("Berlin")},
	"address.region": {Description: langcopy.Phrase{
		En: "State, province or region.",
		De: "Bundesland, Provinz oder Region.",
		Vi: "Tỉnh, bang hoặc vùng.",
	}, Example: langcopy.Phrase{En: "Bavaria", De: "Bayern", Vi: "Bavaria"}},
	"address.postal_code": {Description: langcopy.Phrase{
		En: "Postal code.",
		De: "Postleitzahl.",
		Vi: "Mã bưu chính.",
	}, Example: same("10115")},
	"address.country": {Description: langcopy.Phrase{
		En: "Country, ideally as a two-letter ISO code.",
		De: "Land, am besten als zweistelliger ISO-Code.",
		Vi: "Quốc gia, tốt nhất là mã ISO hai chữ cái.",
	}, Example: same("DE")},
	"first_name": {Description: langcopy.Phrase{
		En: "Given name. Fill first_name and last_name, or full_name.",
		De: "Vorname. Fülle first_name und last_name aus, oder full_name.",
		Vi: "Tên. Điền first_name và last_name, hoặc full_name.",
	}, Example: same("Jo")},
	"last_name": {Description: langcopy.Phrase{
		En: "Family name.",
		De: "Nachname.",
		Vi: "Họ.",
	}, Example: same("Tran")},
	"full_name": {Description: langcopy.Phrase{
		En: "The whole name in one cell, when you do not have it split.",
		De: "Der ganze Name in einer Zelle, wenn er nicht aufgeteilt vorliegt.",
		Vi: "Họ tên đầy đủ trong một ô, khi chưa tách riêng.",
	}, Example: same("Jo Tran")},
	"email": {Description: langcopy.Phrase{
		En: "Email address. Used to find an existing contact or lead when the row has no ref, so uploading again does not make duplicates.",
		De: "E-Mail-Adresse. Hat die Zeile keine Ref, wird damit ein vorhandener Kontakt oder Lead gesucht, sodass erneutes Hochladen keine Duplikate erzeugt.",
		Vi: "Địa chỉ email. Khi dòng không có ref, email dùng để tìm liên hệ hoặc lead đã có, nên tải lên lại không tạo bản trùng.",
	}, Example: same("jo@acme.com")},
	"phone": {Description: langcopy.Phrase{
		En: "Phone number with its country code.",
		De: "Telefonnummer mit Ländervorwahl.",
		Vi: "Số điện thoại kèm mã quốc gia.",
	}, Example: same("+49 30 1234567")},
	"title": {Description: langcopy.Phrase{
		En: "Job title.",
		De: "Berufsbezeichnung.",
		Vi: "Chức danh.",
	}, Example: langcopy.Phrase{En: "Head of Purchasing", De: "Leitung Einkauf", Vi: "Trưởng phòng mua hàng"}},
	"company_name": {Description: langcopy.Phrase{
		En: "The company the lead works for, as text. A lead links to no company record.",
		De: "Das Unternehmen, für das der Lead arbeitet, als Text. Ein Lead ist mit keinem Unternehmen verknüpft.",
		Vi: "Công ty nơi lead làm việc, dạng văn bản. Lead không liên kết với hồ sơ công ty nào.",
	}, Example: same("Acme")},
	"lead.status": {Description: langcopy.Phrase{
		En: "Where the lead stands: new, contacted or engaged. Leave it empty for new. A lead is qualified or disqualified in Margince, so a file cannot set those.",
		De: "Wo der Lead steht: new, contacted oder engaged. Leer bedeutet new. Ein Lead wird in Margince qualifiziert oder disqualifiziert, daher kann eine Datei das nicht setzen.",
		Vi: "Lead đang ở giai đoạn nào: new, contacted hoặc engaged. Để trống nghĩa là new. Lead được xác nhận đủ điều kiện hoặc loại ngay trong Margince, nên tệp không thể đặt hai trạng thái này.",
	}, Example: same("contacted")},
	"lead.origin": {Description: langcopy.Phrase{
		En: "Where the lead came from, as your old CRM words it. A value that matches an origin in Settings by name is filed under it. Any other value is kept as a note on the lead and the origin stays empty. A lead that already has an origin keeps it.",
		De: "Woher der Lead kam, so wie dein altes CRM es nennt. Ein Wert, der dem Namen einer Herkunft in den Einstellungen entspricht, wird dieser zugeordnet. Jeder andere Wert bleibt als Notiz am Lead, die Herkunft bleibt leer. Hat der Lead schon eine Herkunft, behält er sie.",
		Vi: "Lead đến từ đâu, theo cách CRM cũ của bạn ghi. Giá trị trùng tên với một nguồn gốc trong Cài đặt sẽ được xếp vào nguồn gốc đó. Giá trị khác được giữ làm ghi chú trên lead và nguồn gốc để trống. Lead đã có nguồn gốc thì giữ nguyên.",
	}, Example: same("Referral")},
	"lead.origin_detail": {Description: langcopy.Phrase{
		En: "A short note beside the origin, for example the event or who referred the lead. A lead that already has a note keeps it.",
		De: "Eine kurze Notiz zur Herkunft, zum Beispiel die Veranstaltung oder wer den Lead empfohlen hat. Hat der Lead schon eine Notiz, behält er sie.",
		Vi: "Ghi chú ngắn bên cạnh nguồn gốc, ví dụ sự kiện hoặc ai đã giới thiệu lead. Lead đã có ghi chú thì giữ nguyên.",
	}, Example: same("Berlin summit")},
	"contact.company_name": {Description: langcopy.Phrase{
		En: "The company this contact works for, by its exact name. Exactly one company must carry the name, or no link is made and the import preview says why.",
		De: "Das Unternehmen, für das dieser Kontakt arbeitet, mit seinem genauen Namen. Genau ein Unternehmen muss diesen Namen tragen, sonst wird keine Verknüpfung angelegt, und die Importvorschau nennt den Grund.",
		Vi: "Công ty nơi liên hệ này làm việc, theo đúng tên. Phải có đúng một công ty mang tên này, nếu không sẽ không tạo liên kết và bản xem trước lần nhập nêu lý do.",
	}, Example: same("Acme GmbH")},
	"company_ref": {Description: langcopy.Phrase{
		En: "The company this contact works for: the company's ref in this upload or an earlier one, its domain, its exact name, or its Margince ID. If none matches, the import preview reports a broken link.",
		De: "Das Unternehmen, für das dieser Kontakt arbeitet: die Ref des Unternehmens aus diesem oder einem früheren Import, seine Domain, sein genauer Name oder seine Margince-ID. Passt nichts, meldet die Importvorschau eine fehlerhafte Verknüpfung.",
		Vi: "Công ty nơi liên hệ này làm việc: ref của công ty trong lần tải lên này hoặc lần trước, tên miền, tên chính xác, hoặc ID Margince. Nếu không khớp, bản xem trước lần nhập báo liên kết bị hỏng.",
	}, Example: same("CO-0042 · acme.com")},
	"deal.ref": {Description: langcopy.Phrase{
		En: "Your own key for this deal, such as the deal's ID in your old CRM. Every deal row needs one, because two deals may share a name. Upload again with the same ref and source name, and Margince updates the deal.",
		De: "Dein eigener Schlüssel für diesen Deal, etwa die ID des Deals in deinem alten CRM. Jede Deal-Zeile braucht einen, denn zwei Deals können denselben Namen tragen. Lädst du mit derselben Ref und demselben Quellnamen erneut hoch, aktualisiert Margince den Deal.",
		Vi: "Khóa riêng của bạn cho deal này, chẳng hạn ID của deal trong CRM cũ. Mỗi dòng deal đều cần khóa này, vì hai deal có thể trùng tên. Tải lên lại với cùng ref và tên nguồn, Margince sẽ cập nhật deal.",
	}, Example: same("D-1001"), RequiredOnNew: true},
	"deal.name": {Description: langcopy.Phrase{
		En: "The deal's name.",
		De: "Der Name des Deals.",
		Vi: "Tên của deal.",
	}, Example: langcopy.Phrase{En: "Acme pumps renewal", De: "Acme Pumpen Verlängerung", Vi: "Gia hạn máy bơm Acme"}, RequiredOnNew: true},
	"pipeline": {Description: langcopy.Phrase{
		En: "The pipeline's name, as Margince spells it; case does not matter. Leave it empty to use the default pipeline.",
		De: "Der Name der Pipeline, wie Margince ihn schreibt; Groß- und Kleinschreibung zählt nicht. Leer lassen, um die Standard-Pipeline zu nutzen.",
		Vi: "Tên pipeline, đúng như Margince ghi; không phân biệt chữ hoa, chữ thường. Để trống để dùng pipeline mặc định.",
	}, Example: same("Sales")},
	"stage": {Description: langcopy.Phrase{
		En: "The stage's name in that pipeline; case does not matter. A won or lost stage imports the deal as closed.",
		De: "Der Name der Phase in dieser Pipeline; Groß- und Kleinschreibung zählt nicht. Mit einer gewonnenen oder verlorenen Phase wird der Deal als abgeschlossen importiert.",
		Vi: "Tên giai đoạn trong pipeline đó; không phân biệt chữ hoa, chữ thường. Giai đoạn thắng hoặc thua sẽ nhập deal ở trạng thái đã đóng.",
	}, Example: same("Qualified")},
	"amount": {Description: langcopy.Phrase{
		En: "The deal's value as a plain number with a dot for decimals, with no currency sign and no thousands separator. Needs a currency.",
		De: "Der Wert des Deals als einfache Zahl mit Punkt als Dezimaltrennzeichen, ohne Währungszeichen und ohne Tausendertrennzeichen. Braucht eine Währung.",
		Vi: "Giá trị của deal, viết dạng số thuần với dấu chấm cho phần thập phân, không ký hiệu tiền và không dấu phân cách hàng nghìn. Cần có loại tiền.",
	}, Example: same("12000.50")},
	"currency": {Description: langcopy.Phrase{
		En: "The amount's currency, as a three-letter ISO code.",
		De: "Die Währung des Betrags als dreistelliger ISO-Code.",
		Vi: "Loại tiền của số tiền, theo mã ISO ba chữ cái.",
	}, Example: same("EUR")},
	"close_date": {Description: langcopy.Phrase{
		En: "When the deal closes or closed, as YYYY-MM-DD. An open deal's date cannot be in the past; a won or lost deal's may.",
		De: "Wann der Deal abschließt oder abgeschlossen wurde, im Format YYYY-MM-DD. Bei einem offenen Deal darf das Datum nicht in der Vergangenheit liegen, bei einem gewonnenen oder verlorenen schon.",
		Vi: "Ngày deal đóng hoặc đã đóng, theo dạng YYYY-MM-DD. Deal đang mở không được có ngày trong quá khứ; deal thắng hoặc thua thì được.",
	}, Example: same("2026-09-30")},
	"deal.company_ref": {Description: langcopy.Phrase{
		En: "The deal's company: the company's ref in this upload or an earlier one, its domain, its exact name, or its Margince ID. If none matches, the import preview reports a broken link.",
		De: "Das Unternehmen des Deals: die Ref des Unternehmens aus diesem oder einem früheren Import, seine Domain, sein genauer Name oder seine Margince-ID. Passt nichts, meldet die Importvorschau eine fehlerhafte Verknüpfung.",
		Vi: "Công ty của deal: ref của công ty trong lần tải lên này hoặc lần trước, tên miền, tên chính xác, hoặc ID Margince. Nếu không khớp, bản xem trước lần nhập báo liên kết bị hỏng.",
	}, Example: same("CO-0042 · acme.com")},
	"contact_ref": {Description: langcopy.Phrase{
		En: "A contact on the deal: the contact's ref in this upload or an earlier one, their email, or their Margince ID. If none matches, the import preview reports a broken link.",
		De: "Ein Kontakt am Deal: die Ref des Kontakts aus diesem oder einem früheren Import, seine E-Mail-Adresse oder seine Margince-ID. Passt nichts, meldet die Importvorschau eine fehlerhafte Verknüpfung.",
		Vi: "Một liên hệ của deal: ref của liên hệ trong lần tải lên này hoặc lần trước, email, hoặc ID Margince. Nếu không khớp, bản xem trước lần nhập báo liên kết bị hỏng.",
	}, Example: same("P-0007 · jo@acme.com")},
	"deal.description": {Description: langcopy.Phrase{
		En: "A short description of the deal.",
		De: "Eine kurze Beschreibung des Deals.",
		Vi: "Mô tả ngắn về deal.",
	}, Example: langcopy.Phrase{En: "Three pumps and a service contract", De: "Drei Pumpen und ein Wartungsvertrag", Vi: "Ba máy bơm và một hợp đồng bảo trì"}},
	"lost_reason": {Description: langcopy.Phrase{
		En: "Why a lost deal was lost. Needed when the stage is lost; written only when the row creates the deal.",
		De: "Warum ein verlorener Deal verloren ging. Nötig, wenn die Phase verloren ist; wird nur geschrieben, wenn die Zeile den Deal anlegt.",
		Vi: "Lý do deal bị thua. Bắt buộc khi giai đoạn là thua; chỉ được ghi khi dòng tạo deal mới.",
	}, Example: langcopy.Phrase{En: "Chose a cheaper supplier", De: "Günstigeren Anbieter gewählt", Vi: "Chọn nhà cung cấp rẻ hơn"}},
})

// Custom is the guide of a custom field, by its type. A picklist's
// example is replaced by its first option where the caller may read them.
var Custom = map[string]Column{
	fieldcatalog.TypeText: {Description: langcopy.Phrase{
		En: "A custom field. Any text.",
		De: "Ein benutzerdefiniertes Feld. Beliebiger Text.",
		Vi: "Trường tùy chỉnh. Văn bản bất kỳ.",
	}, Example: same("Enterprise")},
	fieldcatalog.TypeNumber: {Description: langcopy.Phrase{
		En: "A custom field. A plain number with a dot for decimals.",
		De: "Ein benutzerdefiniertes Feld. Eine einfache Zahl mit Punkt als Dezimaltrennzeichen.",
		Vi: "Trường tùy chỉnh. Một số thuần, dùng dấu chấm cho phần thập phân.",
	}, Example: same("24")},
	fieldcatalog.TypeDate: {Description: langcopy.Phrase{
		En: "A custom field. A date written as YYYY-MM-DD.",
		De: "Ein benutzerdefiniertes Feld. Ein Datum im Format YYYY-MM-DD.",
		Vi: "Trường tùy chỉnh. Ngày viết theo dạng YYYY-MM-DD.",
	}, Example: same("2026-09-01")},
	fieldcatalog.TypeCurrency: {Description: langcopy.Phrase{
		En: "A custom field. An amount in the field's currency, as a plain number with a dot for decimals, with no currency sign and no thousands separator.",
		De: "Ein benutzerdefiniertes Feld. Ein Betrag in der Währung des Felds als einfache Zahl mit Punkt als Dezimaltrennzeichen, ohne Währungszeichen und ohne Tausendertrennzeichen.",
		Vi: "Trường tùy chỉnh. Số tiền theo loại tiền của trường, viết dạng số thuần với dấu chấm cho phần thập phân, không ký hiệu tiền và không dấu phân cách hàng nghìn.",
	}, Example: same("12000.50")},
	fieldcatalog.TypePicklist: {Description: langcopy.Phrase{
		En: "A custom field. One of the field's allowed values.",
		De: "Ein benutzerdefiniertes Feld. Einer der erlaubten Werte des Felds.",
		Vi: "Trường tùy chỉnh. Một trong các giá trị cho phép của trường.",
	}, Example: same("A")},
	fieldcatalog.TypeMultiselect: {Description: langcopy.Phrase{
		En: "A custom field. One or more of the field's allowed values, separated by semicolons.",
		De: "Ein benutzerdefiniertes Feld. Einer oder mehrere der erlaubten Werte des Felds, durch Semikolons getrennt.",
		Vi: "Trường tùy chỉnh. Một hoặc nhiều giá trị cho phép của trường, phân cách bằng dấu chấm phẩy.",
	}, Example: same("A; B")},
	fieldcatalog.TypeBoolean: {Description: langcopy.Phrase{
		En: "A custom field. Yes or no: true, yes or 1, or false, no or 0.",
		De: "Ein benutzerdefiniertes Feld. Ja oder nein: true, yes oder 1, oder false, no oder 0.",
		Vi: "Trường tùy chỉnh. Có hoặc không: true, yes hoặc 1, hay false, no hoặc 0.",
	}, Example: same("yes")},
}

// KeyRules say how a row is recognised and how files link, in order.
var KeyRules = []langcopy.Phrase{
	{
		En: "A row is known by its ref and the import's source name. Upload a file again with the same refs and source name, and its records are updated, not made twice.",
		De: "Eine Zeile wird an ihrer Ref und am Quellnamen des Imports erkannt. Lädst du eine Datei mit denselben Refs und demselben Quellnamen erneut hoch, werden ihre Datensätze aktualisiert statt doppelt angelegt.",
		Vi: "Mỗi dòng được nhận biết qua ref và tên nguồn của lần nhập. Tải lên lại tệp với cùng ref và tên nguồn, hồ sơ sẽ được cập nhật chứ không bị tạo hai lần.",
	},
	{
		En: "A row without a ref is matched by a company's display name, or by a contact's or lead's email. A deal or an activity needs a ref.",
		De: "Eine Zeile ohne Ref wird bei einem Unternehmen über den Anzeigenamen zugeordnet, bei Kontakt und Lead über die E-Mail-Adresse. Ein Deal oder eine Aktivität braucht eine Ref.",
		Vi: "Dòng không có ref được đối chiếu theo tên hiển thị của công ty, hoặc email của liên hệ hay lead. Deal hoặc hoạt động bắt buộc có ref.",
	},
	{
		En: "Files are imported in the order above, so a contact's company_ref can name a company from the same upload.",
		De: "Die Dateien werden in der obigen Reihenfolge importiert, sodass company_ref eines Kontakts ein Unternehmen aus demselben Import nennen kann.",
		Vi: "Các tệp được nhập theo thứ tự ở trên, nên company_ref của liên hệ có thể trỏ tới công ty trong cùng lần tải lên.",
	},
	{
		En: "Separate several tags or values in one cell with semicolons.",
		De: "Trenne mehrere Tags oder Werte in einer Zelle durch Semikolons.",
		Vi: "Phân cách nhiều thẻ hoặc giá trị trong một ô bằng dấu chấm phẩy.",
	},
}

// same is an example written alike in every language: a key, an address, a
// number.
func same(text string) langcopy.Phrase {
	return langcopy.Phrase{En: text, De: text, Vi: text}
}

// Template is the README's own text, beside the column guides.
var Template = struct {
	Title, Optional, Order, Keys, Allowed, Pipelines, SizeLimit, Formulas, Example, Required langcopy.Phrase
}{
	Title: langcopy.Phrase{
		En: "Margince import template (%s)",
		De: "Margince-Importvorlage (%s)",
		Vi: "Mẫu nhập dữ liệu Margince (%s)",
	},
	Optional: langcopy.Phrase{
		En: "Every file is optional. Fill in only what you have, and delete the files you do not need.",
		De: "Jede Datei ist optional. Fülle nur aus, was du hast, und lösche die Dateien, die du nicht brauchst.",
		Vi: "Tệp nào cũng không bắt buộc. Chỉ điền những gì bạn có và xóa các tệp không cần.",
	},
	Order:   langcopy.Phrase{En: "Import order", De: "Importreihenfolge", Vi: "Thứ tự nhập"},
	Keys:    langcopy.Phrase{En: "Keys", De: "Schlüssel", Vi: "Khóa"},
	Allowed: langcopy.Phrase{En: "Allowed values", De: "Erlaubte Werte", Vi: "Giá trị cho phép"},
	Pipelines: langcopy.Phrase{
		En: "Pipelines and stages (a deal names its pipeline and stage as written here)",
		De: "Pipelines und Phasen (ein Deal nennt Pipeline und Phase so, wie sie hier stehen)",
		Vi: "Quy trình và giai đoạn (giao dịch ghi tên quy trình và giai đoạn đúng như ở đây)",
	},
	SizeLimit: langcopy.Phrase{
		En: "Largest upload: %d MB.",
		De: "Größte Datei zum Hochladen: %d MB.",
		Vi: "Dung lượng tải lên tối đa: %d MB.",
	},
	Formulas: langcopy.Phrase{
		En: "Spreadsheet apps can change what a cell holds: a value that starts with =, + or @, or with a minus sign, may run as a formula, and long numbers or dates may be reformatted. Save as CSV in UTF-8, and check such cells before you upload.",
		De: "Tabellenprogramme können verändern, was in einer Zelle steht: Ein Wert, der mit =, + oder @ oder mit einem Minuszeichen beginnt, kann als Formel ausgeführt werden, und lange Zahlen oder Datumsangaben können umformatiert werden. Speichere als CSV in UTF-8 und prüfe solche Zellen vor dem Hochladen.",
		Vi: "Ứng dụng bảng tính có thể đổi nội dung ô: giá trị bắt đầu bằng =, + hoặc @, hay bằng dấu trừ, có thể chạy như công thức, còn số dài hoặc ngày có thể bị đổi định dạng. Lưu dạng CSV UTF-8 và kiểm tra các ô đó trước khi tải lên.",
	},
	Example:  langcopy.Phrase{En: "Example: %s", De: "Beispiel: %s", Vi: "Ví dụ: %s"},
	Required: langcopy.Phrase{En: "needed on a new row", De: "beim Anlegen nötig", Vi: "bắt buộc khi tạo mới"},
}
