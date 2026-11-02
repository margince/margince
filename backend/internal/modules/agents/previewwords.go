// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The sentences the approval card shows for a preview's effects and for a
// preview it cannot explain, mirrored from the frontend catalogs. The gate in
// backend/gates/approvalpreviewwords_test.go holds each to the card's own.

import (
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// previewStateUnavailable is the state the card reads every unknown one as.
const previewStateUnavailable = "unavailable"

// EffectSentence is one preview effect as the card words it in lang. An effect
// the card has no sentence for reads as its code, as it does on the card.
func EffectSentence(lang textlang.Lang, effect string) string {
	if words, ok := effectSentences[effect]; ok {
		return words.in(lang)
	}
	return effect
}

// PreviewStateSentence is the card's sentence for a preview in state. A state
// the card has no sentence for reads as unavailable.
func PreviewStateSentence(lang textlang.Lang, state string) string {
	words, ok := previewStateSentences[state]
	if !ok {
		words = previewStateSentences[previewStateUnavailable]
	}
	return words.in(lang)
}

// PreviewEffects lists every effect the card has a sentence for, sorted, for
// the gate that holds them to the frontend's.
func PreviewEffects() []string {
	return slices.Sorted(maps.Keys(effectSentences))
}

// PreviewStates lists every state the card has a sentence for, sorted.
func PreviewStates() []string {
	return slices.Sorted(maps.Keys(previewStateSentences))
}

// cardWords is one card sentence in each language the card ships.
type cardWords struct{ en, de, vi string }

func (w cardWords) in(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return w.de
	case textlang.Vietnamese:
		return w.vi
	default:
		return w.en
	}
}

var effectSentences = map[string]cardWords{
	"append_recipient_unsubscribe": {
		en: "Sending adds a recipient-specific unsubscribe link when the recipient has an applicable preference record. The private link is created at execution, not during review.",
		de: "Beim Versand wird ein empfängerbezogener Abmeldelink ergänzt, wenn ein entsprechender Präferenzdatensatz besteht. Der private Link wird erst bei der Ausführung erstellt, nicht bei der Prüfung.",
		vi: "Khi gửi, hệ thống thêm liên kết hủy đăng ký riêng nếu người nhận có bản ghi tùy chọn phù hợp. Liên kết riêng được tạo khi thực hiện, không phải khi xem xét.",
	},
	"apply_company_read": {
		en: "Applies the displayed company fields and evidence. Human-owned evidence remains unchanged; existing column values are retained where shown.",
		de: "Übernimmt die angezeigten Unternehmensfelder und Belege. Menschlich bestätigte Belege bleiben unverändert; vorhandene Feldwerte bleiben wie angezeigt erhalten.",
		vi: "Áp dụng các trường và bằng chứng công ty được hiển thị. Giữ nguyên bằng chứng do người dùng xác nhận và các giá trị hiện có như đã hiển thị.",
	},
	"archive_activity_preserve_content": {
		en: "Archive this activity and remove it from active work. Its content remains stored; previously sent messages are not recalled.",
		de: "Diese Aktivität archivieren und aus der aktiven Arbeit entfernen. Der Inhalt bleibt gespeichert; bereits gesendete Nachrichten werden nicht zurückgerufen.",
		vi: "Lưu trữ hoạt động này và loại khỏi công việc đang hoạt động. Nội dung vẫn được lưu; tin nhắn đã gửi không bị thu hồi.",
	},
	"archive_company_detach_scope": {
		en: "Archive this company and its listed domains, classifications, partner program and relationships. Remove its list memberships and tags. Contacts, deals and conversation history remain.",
		de: "Dieses Unternehmen mit den aufgeführten Domains, Klassifizierungen, dem Partnerprogramm und den Beziehungen archivieren. Listenzugehörigkeiten und Tags entfernen. Kontakte, Deals und Gesprächsverlauf bleiben erhalten.",
		vi: "Lưu trữ công ty này cùng các tên miền, phân loại, chương trình đối tác và mối quan hệ được liệt kê. Xóa tư cách thành viên danh sách và thẻ. Giữ lại liên hệ, cơ hội bán hàng và lịch sử hội thoại.",
	},
	"archive_contact_detach_scope": {
		en: "Archive this contact and retire the listed email addresses, phone numbers, channel bindings and relationships. Remove the listed memberships and tags. Conversation history remains.",
		de: "Diesen Kontakt und die aufgeführten E-Mail-Adressen, Telefonnummern, Kanalverknüpfungen und Beziehungen archivieren. Die aufgeführten Listenmitgliedschaften und Tags entfernen. Der Gesprächsverlauf bleibt erhalten.",
		vi: "Lưu trữ liên hệ này cùng các địa chỉ email, số điện thoại, liên kết kênh và mối quan hệ được liệt kê. Xóa tư cách thành viên danh sách và thẻ được liệt kê. Lịch sử hội thoại được giữ nguyên.",
	},
	"archive_deal_detach_scope": {
		en: "Archive this deal and remove the shown stakeholder links, list memberships and tag assignments. Contacts, lists and tags remain stored.",
		de: "Diesen Deal archivieren und die gezeigten Stakeholder-Verknüpfungen, Listenzugehörigkeiten und Tag-Zuordnungen entfernen. Kontakte, Listen und Tags bleiben gespeichert.",
		vi: "Lưu trữ deal này và xóa các liên kết bên liên quan, tư cách thành viên danh sách và gán nhãn được hiển thị. Liên hệ, danh sách và nhãn vẫn được lưu.",
	},
	"archive_lead_remove_tags": {
		en: "Archives this lead and removes all its tags. The record remains available in archived records.",
		de: "Archiviert diesen Lead und entfernt alle seine Tags. Der Datensatz bleibt unter den archivierten Datensätzen verfügbar.",
		vi: "Lưu trữ lead này và xóa tất cả thẻ của lead. Bạn vẫn có thể xem bản ghi trong danh sách đã lưu trữ.",
	},
	"archive_offer_preserve_lines": {
		en: "Archive this offer and hide it from the active offer list. Its line items remain stored.",
		de: "Dieses Angebot archivieren und aus der aktiven Angebotsliste ausblenden. Seine Positionen bleiben gespeichert.",
		vi: "Lưu trữ báo giá này và ẩn khỏi danh sách báo giá đang hoạt động. Các hạng mục vẫn được lưu.",
	},
	"archive_product_preserve_snapshots": {
		en: "Archive this product and remove it from the active catalog. Existing offer line snapshots remain unchanged.",
		de: "Dieses Produkt archivieren und aus dem aktiven Katalog entfernen. Gespeicherte Angebotspositionen bleiben unverändert.",
		vi: "Lưu trữ sản phẩm này và loại khỏi danh mục đang hoạt động. Các bản chụp hạng mục báo giá hiện có không thay đổi.",
	},
	"archive_project_detach_scope": {
		en: "Archive this project and remove the listed stakeholder links, list memberships and tags. Activities, deals and company links remain.",
		de: "Dieses Projekt archivieren und die aufgeführten Beteiligten-Verknüpfungen, Listenmitgliedschaften und Tags entfernen. Aktivitäten, Deals und Firmenverknüpfungen bleiben erhalten.",
		vi: "Lưu trữ dự án này và xóa các liên kết bên liên quan, tư cách thành viên danh sách và thẻ được liệt kê. Hoạt động, giao dịch và liên kết công ty được giữ nguyên.",
	},
	"archive_relationship_no_primary": {
		en: "Leave the contact without a primary employer. No employer is chosen automatically.",
		de: "Der Kontakt bleibt ohne primären Arbeitgeber. Es wird kein Arbeitgeber automatisch ausgewählt.",
		vi: "Để liên hệ không có nhà tuyển dụng chính. Không tự động chọn nhà tuyển dụng.",
	},
	"archive_relationship_preserve_records": {
		en: "Archive this relationship link. The linked records remain stored.",
		de: "Diese Verknüpfung archivieren. Die verknüpften Datensätze bleiben gespeichert.",
		vi: "Lưu trữ liên kết quan hệ này. Các bản ghi được liên kết vẫn được lưu.",
	},
	"archive_relationship_promote_employer": {
		en: "Make the shown remaining employment the contact’s primary employer.",
		de: "Die gezeigte verbleibende Anstellung als primären Arbeitgeber des Kontakts festlegen.",
		vi: "Đặt công ty của quan hệ công tác còn lại được hiển thị làm nhà tuyển dụng chính của liên hệ.",
	},
	"archive_template_preserve_offers": {
		en: "Archive this template and remove it from template selection, including its use as a default. Existing offers keep their template reference.",
		de: "Diese Vorlage archivieren und aus der Vorlagenauswahl entfernen, auch als Standardvorlage. Bestehende Angebote behalten ihren Vorlagenverweis.",
		vi: "Lưu trữ mẫu này và loại khỏi danh sách chọn mẫu, kể cả vai trò mẫu mặc định. Các báo giá hiện có giữ tham chiếu tới mẫu.",
	},
	"assign_offer_number": {
		en: "Assign the offer number when the draft is created.",
		de: "Die Angebotsnummer wird beim Erstellen des Entwurfs vergeben.",
		vi: "Cấp số báo giá khi tạo bản nháp.",
	},
	"assign_project_key": {
		en: "Assign a project key when the project is created.",
		de: "Bei der Erstellung einen Projektschlüssel vergeben.",
		vi: "Cấp mã dự án khi tạo dự án.",
	},
	"attach_task_recipients": {
		en: "Link these recipients to the existing task.",
		de: "Diese Kontakte mit der bestehenden Aufgabe verknüpfen.",
		vi: "Liên kết các liên hệ nhận với công việc hiện có.",
	},
	"authorize_calendar_invitation": {
		en: "Authorizes the proposing host to reserve the slot and queue this calendar invitation to the displayed address. A private reschedule/cancel link is appended to the invitation. Video and reminder settings are shown above. Current calendar access, availability and recipient permissions are checked when executed. Provider confirmation does not prove the guest accepted or received the invitation.",
		de: "Autorisiert die vorschlagende organisierende Person, den Zeitraum zu reservieren und diese Kalendereinladung an die angezeigte Adresse zum Versand einzureihen. Der Einladung wird ein privater Link zum Verschieben oder Absagen hinzugefügt. Video- und Erinnerungseinstellungen stehen oben. Kalenderzugriff, Verfügbarkeit und Empfangsberechtigungen werden bei der Ausführung geprüft. Eine Bestätigung des Anbieters belegt weder die Annahme noch den Empfang durch den Gast.",
		vi: "Cho phép người chủ trì đề xuất giữ khung giờ và xếp lời mời lịch này vào hàng đợi gửi tới địa chỉ hiển thị. Một liên kết riêng để đổi lịch hoặc hủy được thêm vào lời mời. Cài đặt video và nhắc nhở được hiển thị ở trên. Quyền truy cập lịch, thời gian trống và quyền gửi tới người nhận được kiểm tra khi thực hiện. Xác nhận từ nhà cung cấp không chứng minh khách đã chấp nhận hoặc nhận được lời mời.",
	},
	"authorize_channel_reply": {
		en: "Authorizes the proposing caller to reply once on this conversation to the displayed channel account, with the full message and listed files. Approval does not send immediately. Current channel availability, recipient permissions and file checks still apply.",
		de: "Autorisiert den vorschlagenden Aufrufer, einmal in dieser Unterhaltung mit dem vollständigen Text und den aufgelisteten Dateien an das angezeigte Kanalkonto zu antworten. Die Freigabe versendet noch nichts. Kanalverfügbarkeit, Empfangsberechtigungen und Dateien werden bei der Ausführung erneut geprüft.",
		vi: "Cho phép người đề xuất trả lời một lần trong cuộc hội thoại này tới tài khoản kênh hiển thị, với toàn bộ nội dung và các tệp đã liệt kê. Phê duyệt không gửi ngay. Vẫn phải kiểm tra khả năng gửi của kênh, quyền gửi tới người nhận và các tệp tại thời điểm thực hiện.",
	},
	"authorize_message_send": {
		en: "Authorizes the proposing caller to send this message once. Approval does not send it immediately or add your signature. Sending still requires current permissions and consent checks.",
		de: "Erlaubt dem vorschlagenden Aufrufer, diese Nachricht einmal zu senden. Die Freigabe sendet sie nicht sofort und fügt deine Signatur nicht hinzu. Beim Senden werden die aktuellen Berechtigungen und Einwilligungen erneut geprüft.",
		vi: "Cho phép bên đề xuất gửi tin nhắn này một lần. Phê duyệt không gửi ngay và không thêm chữ ký của bạn. Khi gửi, hệ thống vẫn kiểm tra quyền truy cập và sự đồng ý hiện tại.",
	},
	"authorize_website_read": {
		en: "Authorizes the proposing caller to read this website using configured external services. Reads may use AI and incur usage charges.",
		de: "Erlaubt dem vorschlagenden Aufrufer, diese Website mit den konfigurierten externen Diensten zu lesen. Dabei können KI und nutzungsabhängige Gebühren anfallen.",
		vi: "Cho phép bên đề xuất đọc trang web này bằng các dịch vụ bên ngoài đã cấu hình. Việc đọc có thể sử dụng AI và phát sinh phí sử dụng.",
	},
	"classify_correspondence": {
		en: "Classify linked correspondence as business records.",
		de: "Verknüpfte Korrespondenz als Geschäftsbriefe einordnen.",
		vi: "Phân loại thư từ liên quan là hồ sơ kinh doanh.",
	},
	"create_activity_record": {
		en: "File the activity without sending a message or calendar invitation",
		de: "Aktivität ablegen, ohne Nachricht oder Kalendereinladung zu senden",
		vi: "Lưu hoạt động mà không gửi tin nhắn hoặc lời mời lịch",
	},
	"create_company_record": {
		en: "Authorizes the proposing caller to create this company once with the displayed values. Current permissions and duplicate identities are checked again.",
		de: "Erlaubt dem vorschlagenden Aufrufer, dieses Unternehmen einmal mit den angezeigten Werten anzulegen. Aktuelle Berechtigungen und doppelte Identitäten werden erneut geprüft.",
		vi: "Cho phép bên đề xuất tạo công ty này một lần với các giá trị hiển thị. Quyền hiện tại và danh tính trùng được kiểm tra lại.",
	},
	"create_contact_employer": {
		en: "Create the missing company and link this contact as an employee.",
		de: "Das fehlende Unternehmen erstellen und als Arbeitgeber dieses Kontakts verknüpfen.",
		vi: "Tạo công ty còn thiếu và liên kết liên hệ này làm nhân viên.",
	},
	"create_contact_record": {
		en: "Authorizes the proposing caller to create this contact once with the displayed values. Current permissions and duplicate identities are checked again.",
		de: "Erlaubt dem vorschlagenden Aufrufer, diesen Kontakt einmal mit den angezeigten Werten anzulegen. Aktuelle Berechtigungen und doppelte Identitäten werden erneut geprüft.",
		vi: "Cho phép bên đề xuất tạo bản ghi liên hệ này một lần với các giá trị hiển thị. Quyền hiện tại và danh tính trùng được kiểm tra lại.",
	},
	"create_deal_record": {
		en: "Create the deal in the shown pipeline and stage, with its initial stage history.",
		de: "Den Deal in der angezeigten Pipeline und Phase mit dem ersten Eintrag im Phasenverlauf erstellen.",
		vi: "Tạo cơ hội trong quy trình và giai đoạn được hiển thị, kèm lịch sử giai đoạn ban đầu.",
	},
	"create_lead_record": {
		en: "Authorizes the proposing caller to create this lead once with the displayed values. Creation checks current permissions and duplicate identities again.",
		de: "Erlaubt dem vorschlagenden Aufrufer, diesen Lead einmal mit den angezeigten Werten anzulegen. Beim Anlegen werden Berechtigungen und doppelte Identitäten erneut geprüft.",
		vi: "Cho phép bên đề xuất tạo lead này một lần với các giá trị hiển thị. Khi tạo, quyền và danh tính trùng được kiểm tra lại.",
	},
	"create_offer_draft": {
		en: "Create a draft offer with the shown terms, line items and totals. No offer is sent.",
		de: "Einen Angebotsentwurf mit den gezeigten Bedingungen, Positionen und Summen erstellen. Es wird kein Angebot versendet.",
		vi: "Tạo bản nháp báo giá với các điều khoản, hạng mục và tổng tiền được hiển thị. Chưa gửi báo giá.",
	},
	"create_offer_template": {
		en: "Create an offer template with the shown header and footer.",
		de: "Eine Angebotsvorlage mit der angezeigten Kopf- und Fußzeile erstellen.",
		vi: "Tạo mẫu báo giá với nội dung đầu trang và chân trang được hiển thị.",
	},
	"create_product_record": {
		en: "Create the catalog product with the shown price and billing terms. Existing offer lines keep their prices.",
		de: "Das Katalogprodukt mit dem angezeigten Preis und den Abrechnungsbedingungen erstellen. Bestehende Angebotspositionen behalten ihre Preise.",
		vi: "Tạo sản phẩm trong danh mục với giá và điều khoản thanh toán được hiển thị. Các dòng báo giá hiện có giữ nguyên giá.",
	},
	"create_project_record": {
		en: "Create the project, link the shown company as its customer, and record the initial phase.",
		de: "Das Projekt erstellen, das angezeigte Unternehmen als Kunden verknüpfen und die Anfangsphase dokumentieren.",
		vi: "Tạo dự án, liên kết công ty được hiển thị làm khách hàng và ghi lại giai đoạn ban đầu.",
	},
	"create_workspace_visible": {
		en: "Makes the new record available to colleagues who have permission to see it.",
		de: "Der neue Datensatz ist mit passenden Leserechten sichtbar.",
		vi: "Bản ghi mới hiển thị trong tổ chức theo quyền truy cập bản ghi.",
	},
	"fill_empty_lead_fields": {
		en: "Fills only the empty lead fields shown above. Existing values and the email address stay unchanged; no second lead is created.",
		de: "Ergänzt nur die oben gezeigten leeren Lead-Felder. Vorhandene Werte und die E-Mail-Adresse bleiben erhalten; es wird kein zweiter Lead angelegt.",
		vi: "Chỉ điền các trường trống của lead được hiển thị ở trên. Giá trị hiện có và địa chỉ email được giữ nguyên; không tạo hồ sơ thứ hai.",
	},
	"geocode_if_enabled": {
		en: "If address geocoding is enabled, queues a coordinate lookup for the displayed address.",
		de: "Wenn die Adress-Geokodierung aktiviert ist, wird eine Koordinatenabfrage für die angezeigte Adresse eingeplant.", //nolint:misspell // "Adress-" is German, as the card's catalog spells it.
		vi: "Nếu tính năng mã hóa địa lý được bật, hệ thống sẽ xếp hàng tra cứu tọa độ cho địa chỉ hiển thị.",
	},
	"import_close_deal": {
		en: "Close the deal using the imported history",
		de: "Deal anhand des importierten Verlaufs abschließen",
		vi: "Đóng cơ hội theo lịch sử đã nhập",
	},
	"marketing_confirmation_unavailable": {
		en: "The current contact or consent details prevent a marketing confirmation request. Booking can continue without sending it or granting marketing consent.",
		de: "Die aktuellen Kontakt- oder Einwilligungsdaten verhindern eine Marketingbestätigung. Die Buchung kann ohne Versand und ohne Marketingeinwilligung fortgesetzt werden.",
		vi: "Thông tin liên hệ hoặc sự đồng ý hiện tại ngăn yêu cầu xác nhận tiếp thị. Vẫn có thể đặt lịch mà không gửi yêu cầu hoặc cấp sự đồng ý tiếp thị.",
	},
	"member_connectors_reconnect": {
		en: "Connected accounts must be reconnected under the surviving member.",
		de: "Verbundene Konten müssen beim verbleibenden Mitglied neu verbunden werden.",
		vi: "Cần kết nối lại các dịch vụ dưới thành viên giữ lại.",
	},
	"member_crm_private_records_move": {
		en: "Owned CRM records, including private contacts and companies, become visible to the receiving colleague.",
		de: "Zugewiesene CRM-Datensätze, auch private Kontakte und Unternehmen, werden für das empfangende Mitglied sichtbar.",
		vi: "Đồng nghiệp nhận có thể xem các hồ sơ CRM được bàn giao, kể cả liên hệ và công ty riêng tư.",
	},
	"member_email_reserved": {
		en: "Removal cannot be undone here. The original email stays reserved; use Deactivate if this colleague may return.",
		de: "Das Entfernen lässt sich hier nicht rückgängig machen. Die ursprüngliche E-Mail-Adresse bleibt reserviert. Falls das Mitglied zurückkehren könnte, Deaktivieren verwenden.",
		vi: "Không thể hoàn tác việc xóa tại đây. Email ban đầu vẫn được giữ riêng. Dùng Tắt nếu đồng nghiệp này có thể quay lại.",
	},
	"member_original_attribution": {
		en: "Original names remain in history.",
		de: "Die ursprünglichen Namen bleiben im Verlauf erhalten.",
		vi: "Lịch sử giữ nguyên tên thành viên thực hiện ban đầu.",
	},
	"member_private_alerts_archived": {
		en: "Personal alerts are archived.",
		de: "Persönliche Hinweise werden archiviert.",
		vi: "Các cảnh báo cá nhân được chuyển vào kho lưu trữ.",
	},
	"member_private_audiences_retained": {
		en: "Mail and activities keep their own audience. Activity already shared with a transferred record’s readers may become discoverable to the recipient.",
		de: "E-Mails und Aktivitäten behalten ihre Freigaben. Bereits freigegebene Aktivitäten können mit übertragenen Datensätzen für das neu zuständige Mitglied sichtbar werden.",
		vi: "Email và hoạt động giữ nguyên phạm vi chia sẻ. Thành viên nhận có thể tìm thấy hoạt động đã chia sẻ với các thành viên có quyền xem hồ sơ được bàn giao.",
	},
	"member_source_signed_out": {
		en: "The removed identity is signed out everywhere.",
		de: "Das entfernte Konto wird überall abgemeldet.",
		vi: "Tài khoản bị xóa sẽ đăng xuất ở mọi nơi.",
	},
	"member_survivor_access": {
		en: "The survivor keeps its roles, teams and sign-in settings, and retains the merged identity’s personal access and correspondence.",
		de: "Das verbleibende Mitglied behält Rollen, Teams und Anmeldeeinstellungen und erhält den persönlichen Zugriff und die Korrespondenz des zusammengeführten Kontos.",
		vi: "Thành viên giữ lại giữ nguyên vai trò, nhóm và thiết lập đăng nhập, đồng thời kế thừa quyền truy cập cá nhân và thư từ của tài khoản được gộp.",
	},
	"member_survivor_inactive": {
		en: "The surviving member is deactivated. Reactivate it to restore sign-in and team assignment.",
		de: "Das verbleibende Mitglied ist deaktiviert. Für Anmeldung und Teamzuordnung wieder aktivieren.",
		vi: "Thành viên giữ lại đang bị tắt. Bật lại để đăng nhập và thêm vào nhóm.",
	},
	"merge_company_preserve_receipts": {
		en: "Partner-program and tax-verification records move only when the survivor has none. Otherwise the source copies remain with the archived company.",
		de: "Partnerprogramm- und Steuerprüfungsnachweise werden nur übernommen, wenn das Ziel keine hat. Andernfalls bleiben die Quellkopien beim archivierten Unternehmen.",
		vi: "Hồ sơ chương trình đối tác và xác minh thuế chỉ được chuyển khi công ty giữ lại chưa có. Nếu đã có, bản nguồn vẫn thuộc công ty được lưu trữ.",
	},
	"merge_company_private_readings": {
		en: "Each reader keeps their existing survivor summary when both companies have one. Suggestions dismissed on the removed company may be offered again on the survivor.",
		de: "Hat eine Person Zusammenfassungen für beide Unternehmen, bleibt ihre Zusammenfassung des Ziels erhalten. Für die Quelle ausgeblendete Vorschläge können beim Ziel erneut erscheinen.",
		vi: "Nếu một người có bản tóm tắt cho cả hai công ty, bản của công ty giữ lại được giữ nguyên. Đề xuất đã bỏ qua trên công ty bị loại bỏ có thể xuất hiện lại trên công ty giữ lại.",
	},
	"merge_company_reconcile_lookups": {
		en: "Website lookups move to the survivor; duplicate active lookups are canceled. Background company scans for the removed company stop.",
		de: "Website-Abfragen wechseln zum Ziel; doppelte laufende Abfragen werden abgebrochen. Hintergrundprüfungen für das entfernte Unternehmen enden.",
		vi: "Các lượt tra cứu trang web được chuyển sang công ty giữ lại; lượt đang chạy trùng lặp bị hủy. Việc kiểm tra nền cho công ty bị loại bỏ dừng lại.",
	},
	"merge_records_release_source": {
		en: "Moves the displayed records and values to the survivor and archives the source. Existing survivor values take precedence where shown.",
		de: "Übernimmt die angezeigten Datensätze und Werte in das Ziel und archiviert die Quelle. Vorhandene Zielwerte haben an den angezeigten Stellen Vorrang.",
		vi: "Chuyển các hồ sơ và giá trị được hiển thị sang hồ sơ giữ lại rồi lưu trữ hồ sơ nguồn. Giá trị hiện có của hồ sơ giữ lại được ưu tiên ở những mục được chỉ rõ.",
	},
	"merge_tags_release_source": {
		en: "Replace the source tag on the shown records with the surviving tag. Where both tags are present, remove only the duplicate source assignment. Archive the source tag and release its name for reuse; it is not retained as an alias.",
		de: "Das Quell-Tag auf den gezeigten Datensätzen durch das verbleibende Tag ersetzen. Wenn beide Tags vorhanden sind, nur die doppelte Quell-Zuordnung entfernen. Das Quell-Tag archivieren und seinen Namen zur erneuten Verwendung freigeben; er bleibt nicht als Alias erhalten.",
		vi: "Thay nhãn nguồn trên các bản ghi được hiển thị bằng nhãn giữ lại. Nếu đã có cả hai nhãn, chỉ xóa gán nhãn nguồn trùng lặp. Lưu trữ nhãn nguồn và cho phép dùng lại tên; tên không được giữ làm bí danh.",
	},
	"partner_margin_tier_unset": {
		en: "Margin tier: not set. A human sets it on the company’s Partner tab.",
		de: "Margenstufe: nicht gesetzt. Ein Mensch legt sie im Partner-Tab des Unternehmens fest.",
		vi: "Mức hoa hồng đối tác: chưa đặt. Một thành viên đặt mức này ở tab Đối tác của công ty.",
	},
	"preserve_existing_consent": {
		en: "The existing consent decision stays unchanged; no new consent proof is recorded.",
		de: "Die bestehende Einwilligungsentscheidung bleibt unverändert; es wird kein neuer Nachweis erfasst.",
		vi: "Giữ nguyên quyết định đồng ý hiện có; không ghi bằng chứng đồng ý mới.",
	},
	"promote_create_contact": {
		en: "Creates a contact with the displayed details.",
		de: "Erstellt einen Kontakt mit den angezeigten Angaben.",
		vi: "Tạo liên hệ với thông tin đã hiển thị.",
	},
	"promote_fill_company_industry": {
		en: "Sets the displayed industry on the linked company, which has none yet.",
		de: "Trägt die angezeigte Branche beim verknüpften Unternehmen ein, das noch keine hat.",
		vi: "Ghi ngành đã hiển thị vào công ty được liên kết, vì công ty chưa có ngành.",
	},
	"promote_fill_contact": {
		en: "Adds the displayed missing details to the existing contact. Keeps its existing name, title and primary phone.",
		de: "Ergänzt die angezeigten fehlenden Angaben am bestehenden Kontakt. Vorhandener Name, Position und Hauptnummer bleiben erhalten.",
		vi: "Bổ sung thông tin còn thiếu đã hiển thị vào liên hệ hiện có. Giữ tên, chức danh và số điện thoại chính hiện có.",
	},
	"promote_lead_archive_source": {
		en: "Qualifies and archives this lead, linking it to the contact. Keeps the original consent evidence on the lead.",
		de: "Qualifiziert und archiviert diesen Lead und verknüpft ihn mit dem Kontakt. Die ursprünglichen Einwilligungsbelege bleiben am Lead.",
		vi: "Xác định lead đủ điều kiện, lưu trữ lead và liên kết với liên hệ. Giữ bằng chứng đồng ý ban đầu ở lead.",
	},
	"promote_move_consent_links": {
		en: "Moves the listed unsubscribe links and communication bases to the contact. Their scope, expiry and revocation remain unchanged.",
		de: "Verschiebt die aufgeführten Abmeldelinks und Kommunikationsgrundlagen zum Kontakt. Geltungsbereich, Ablauf und Widerruf bleiben unverändert.",
		vi: "Chuyển các liên kết hủy đăng ký và căn cứ liên lạc đã liệt kê sang liên hệ. Giữ nguyên phạm vi, thời hạn và trạng thái thu hồi.",
	},
	"promote_open_deal_seat_contact": {
		en: "Opens the displayed deal and adds the contact as a stakeholder. The lead links to this deal.",
		de: "Eröffnet den angezeigten Deal und fügt den Kontakt als Beteiligten hinzu. Der Lead wird mit diesem Deal verknüpft.",
		vi: "Mở cơ hội đã hiển thị và thêm liên hệ làm bên liên quan. Lead được liên kết với cơ hội này.",
	},
	"promote_propose_company": {
		en: "Creates no company from the typed name. The contact shows the name as a company to link or create.",
		de: "Legt aus dem eingetippten Namen kein Unternehmen an. Der Kontakt zeigt den Namen als Unternehmen zum Verknüpfen oder Anlegen.",
		vi: "Không tạo công ty từ tên đã nhập. Liên hệ hiển thị tên này như một công ty cần liên kết hoặc tạo.",
	},
	"promote_queue_duplicate_review": {
		en: "Creates a separate contact and queues the possible duplicate for review.",
		de: "Erstellt einen separaten Kontakt und stellt die mögliche Dublette zur Prüfung bereit.",
		vi: "Tạo một liên hệ riêng và đưa cặp có thể trùng vào danh sách xem xét.",
	},
	"queue_duplicate_review": {
		en: "Creates a duplicate candidate for review alongside the new record.",
		de: "Legt neben dem neuen Datensatz einen Dublettenvorschlag zur Prüfung an.",
		vi: "Tạo đề xuất trùng lặp để kiểm tra cùng với bản ghi mới.",
	},
	"queue_vat_check": {
		en: "Request a VAT-number check if verification is configured.",
		de: "Prüfung der Umsatzsteuer-ID anfordern, sofern eingerichtet.",
		vi: "Yêu cầu kiểm tra mã số VAT nếu dịch vụ xác minh được cấu hình.",
	},
	"rearm_held_message": {
		en: "Schedules the displayed held message to be retried after the delay shown. Current permissions, recipients and communication rules are checked again before sending.",
		de: "Plant einen erneuten Versuch der angehaltenen Nachricht nach der angegebenen Wartezeit. Berechtigungen, Empfangende und Kommunikationsregeln werden vor dem Versand erneut geprüft.",
		vi: "Lên lịch thử gửi lại tin nhắn đang giữ sau khoảng chờ hiển thị. Quyền, người nhận và quy tắc liên lạc hiện tại được kiểm tra lại trước khi gửi.",
	},
	"record_confirmation_only": {
		en: "Record this confirmation and complete the waiting automation run. No record update or message send is authorized.",
		de: "Diese Bestätigung erfassen und den wartenden Automatisierungslauf abschließen. Es wird keine Datensatzänderung und kein Nachrichtenversand freigegeben.",
		vi: "Ghi nhận xác nhận này và hoàn tất lượt tự động hóa đang chờ. Không cho phép cập nhật bản ghi hoặc gửi tin nhắn.",
	},
	"record_contact_acquisition": {
		en: "Records the displayed acquisition history. This does not grant permission to send messages.",
		de: "Erfasst die angezeigten Angaben zur Herkunft. Das erlaubt noch keinen Nachrichtenversand.",
		vi: "Ghi lại thông tin nguồn thu thập hiển thị. Việc này không cấp quyền gửi tin nhắn.",
	},
	"record_meeting_only": {
		en: "Authorizes recording a meeting on the linked records. No calendar invitation or email is sent. Supplied attendee addresses are not stored on the meeting. The host must still have a free slot when it is recorded.",
		de: "Erlaubt das Erfassen eines Termins bei den verknüpften Einträgen. Es wird keine Kalendereinladung oder E-Mail versendet. Angegebene Adressen der Teilnehmenden werden nicht am Termin gespeichert. Der Zeitraum muss beim Erfassen für die organisierende Person noch verfügbar sein.",
		vi: "Cho phép ghi lại cuộc họp trên các bản ghi được liên kết. Không gửi lời mời lịch hoặc email. Địa chỉ người tham dự được cung cấp không được lưu trong cuộc họp. Khung giờ của người chủ trì phải còn trống khi ghi lại.",
	},
	"record_meeting_with_consent": {
		en: "Authorizes recording the meeting and capturing the displayed transactional consent before reserving the slot. An existing consent decision is preserved. Consent may be recorded even if the slot is no longer available. No calendar invitation is sent.",
		de: "Autorisiert das Erfassen des Termins und der angezeigten transaktionalen Einwilligung vor der Zeitreservierung. Eine bestehende Entscheidung bleibt erhalten. Die Einwilligung kann erfasst werden, auch wenn der Zeitraum nicht mehr frei ist. Es wird keine Kalendereinladung versendet.",
		vi: "Cho phép ghi cuộc hẹn và sự đồng ý giao dịch hiển thị trước khi giữ khung giờ. Quyết định đồng ý hiện có được giữ nguyên. Sự đồng ý có thể được ghi ngay cả khi khung giờ không còn trống. Không gửi lời mời lịch.",
	},
	"relink_preserve_other_links": {
		en: "Other record links remain. Participants tied to replaced contact links move to the destination contact; matching participant rows merge.",
		de: "Andere Verknüpfungen bleiben bestehen. Beteiligte ersetzter Kontaktverknüpfungen wechseln zum Zielkontakt; gleiche Einträge werden zusammengeführt.",
		vi: "Các liên kết khác được giữ nguyên. Người tham gia thuộc liên kết liên hệ bị thay thế được chuyển sang liên hệ đích; các mục trùng khớp được gộp.",
	},
	"relink_project_retention": {
		en: "Filing under this project records a commercial correspondence retention basis. Moving the activity later does not remove that basis.",
		de: "Die Ablage unter diesem Projekt begründet die Aufbewahrung als Geschäftskorrespondenz. Eine spätere Verschiebung hebt diesen Grund nicht auf.",
		vi: "Gắn hoạt động với dự án này tạo căn cứ lưu giữ thư từ thương mại. Chuyển hoạt động sau đó không xóa căn cứ này.",
	},
	"request_marketing_confirmation": {
		en: "Authorizes an attempt to request marketing confirmation at the displayed address. Delivery depends on the configured mail service and current consent checks. This does not subscribe the contact. The submitted marketing wording is not stored as proof; confirmation uses the wording on the confirmation page.",
		de: "Autorisiert den Versuch, eine Marketingbestätigung an die angezeigte Adresse anzufordern. Die Zustellung hängt vom konfigurierten Maildienst und aktuellen Einwilligungsprüfungen ab. Der Kontakt wird dadurch nicht angemeldet. Der eingereichte Marketingtext wird nicht als Nachweis gespeichert; maßgeblich ist der Text auf der Bestätigungsseite.",
		vi: "Cho phép thử gửi yêu cầu xác nhận tiếp thị tới địa chỉ hiển thị. Việc gửi phụ thuộc vào dịch vụ thư đã cấu hình và kiểm tra sự đồng ý hiện tại. Hành động này không đăng ký liên hệ. Nội dung tiếp thị đã gửi không được lưu làm bằng chứng; xác nhận sử dụng nội dung trên trang xác nhận.",
	},
	"resolve_contact_employer": {
		en: "Resolves the employer from the company or work address, reuses or creates the company, and links employment. Uncertain domains wait for review; eligible enrichment follows company settings.",
		de: "Ermittelt den Arbeitgeber aus dem Unternehmen oder der geschäftlichen Adresse, verwendet ein vorhandenes Unternehmen oder legt es an und verknüpft die Beschäftigung. Unklare Domains warten auf Prüfung; die Anreicherung folgt den Einstellungen des Unternehmens.",
		vi: "Xác định nơi làm việc từ công ty hoặc địa chỉ công việc, dùng lại hoặc tạo công ty rồi liên kết việc làm. Tên miền chưa rõ sẽ chờ xem xét; việc bổ sung thông tin tuân theo cài đặt của công ty.",
	},
	"restore_lead_archive_contact": {
		en: "Restores the lead as Engaged and removes its qualification links. Archives the contact, its email addresses, phone numbers, channel identities and relationships; removes its tags and list memberships. Activities stay where they are.",
		de: "Stellt den Lead als Engagiert wieder her und entfernt seine Qualifizierungsverknüpfungen. Archiviert den Kontakt, seine E-Mail-Adressen, Telefonnummern, Kanalidentitäten und Beziehungen; entfernt seine Tags und Listenzugehörigkeiten. Aktivitäten bleiben unverändert zugeordnet.",
		vi: "Khôi phục lead về trạng thái Đã tương tác và xóa các liên kết xác định đủ điều kiện. Lưu trữ liên hệ, địa chỉ email, số điện thoại, danh tính kênh và các mối quan hệ; xóa thẻ và tư cách thành viên danh sách. Hoạt động giữ nguyên liên kết.",
	},
	"restore_lead_preserve_contact": {
		en: "Restores the lead as Engaged and removes its qualification links. Keeps the existing contact and its history; removes the link back to this lead. Activities stay where they are.",
		de: "Stellt den Lead als Engagiert wieder her und entfernt seine Qualifizierungsverknüpfungen. Behält den bestehenden Kontakt und seine Historie; entfernt die Rückverknüpfung zu diesem Lead. Aktivitäten bleiben unverändert zugeordnet.",
		vi: "Khôi phục lead về trạng thái Đã tương tác và xóa các liên kết xác định đủ điều kiện. Giữ liên hệ hiện có và lịch sử; xóa liên kết ngược đến lead này. Hoạt động giữ nguyên liên kết.",
	},
	"reuse_contact_employer": {
		en: "Link this contact to the existing company.",
		de: "Diesen Kontakt mit dem bestehenden Unternehmen verknüpfen.",
		vi: "Liên kết liên hệ này với công ty hiện có.",
	},
	"reuse_existing_record": {
		en: "The source key already identifies this record. The call returns it without creating another record.",
		de: "Der Quellschlüssel bezeichnet bereits diesen Datensatz. Der Aufruf gibt ihn zurück, ohne einen weiteren anzulegen.",
		vi: "Khóa nguồn đã xác định bản ghi này. Lệnh trả về bản ghi đó mà không tạo thêm bản ghi.",
	},
	"send_without_human_signature": {
		en: "Sends without a human display name or personal signature. Approval does not make the reviewer the author.",
		de: "Versand ohne menschlichen Anzeigenamen oder persönliche Signatur. Die Freigabe macht die prüfende Person nicht zur absendenden Person.",
		vi: "Gửi không kèm tên hiển thị của cá nhân hoặc chữ ký cá nhân. Việc phê duyệt không biến người xem xét thành tác giả.",
	},
	"set_template_locale_default": {
		en: "Set this template as the default for the shown language.",
		de: "Diese Vorlage als Standard für die angezeigte Sprache festlegen.",
		vi: "Đặt mẫu này làm mặc định cho ngôn ngữ được hiển thị.",
	},
	"start_project_delivery": {
		en: "Start delivery on eligible linked projects.",
		de: "Bei berechtigten verknüpften Projekten die Umsetzung starten.",
		vi: "Bắt đầu triển khai các dự án liên quan đủ điều kiện.",
	},
}

var previewStateSentences = map[string]cardWords{
	"incomplete": {
		en: "The full change cannot be shown. Request a new proposal before approving.",
		de: "Die vollständige Änderung kann nicht angezeigt werden. Fordere vor der Freigabe einen neuen Vorschlag an.",
		vi: "Không thể hiển thị đầy đủ thay đổi. Yêu cầu đề xuất mới trước khi duyệt.",
	},
	"needs_wider_sight": {
		en: "This change touches records you cannot open. A colleague who can see them must decide it.",
		de: "Diese Änderung betrifft Datensätze, die du nicht öffnen kannst. Entscheiden muss jemand, der sie sehen kann.",
		vi: "Thay đổi này chạm đến các bản ghi bạn không mở được. Một đồng nghiệp xem được chúng phải quyết định.",
	},
	"stale": {
		en: "This record changed after the proposal. Request a new proposal before approving.",
		de: "Dieser Eintrag wurde nach dem Vorschlag geändert. Fordere vor der Freigabe einen neuen Vorschlag an.",
		vi: "Hồ sơ đã thay đổi sau khi đề xuất được tạo. Yêu cầu đề xuất mới trước khi duyệt.",
	},
	"unavailable": {
		en: "Approval context is unavailable. Reload or request a new proposal.",
		de: "Der Freigabekontext ist nicht verfügbar. Lade die Seite neu oder fordere einen neuen Vorschlag an.",
		vi: "Ngữ cảnh phê duyệt không có sẵn. Tải lại hoặc yêu cầu đề xuất mới.",
	},
}
