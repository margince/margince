// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The codes a report carries and their sentences in each language. The English
// sentence is also what a reader that knows no code is shown, so it is the
// wording the report always had.

// The codes. The contract's ImportReasonCode enum is this list.
const (
	ReasonEmptyPayload            ReasonCode = "empty_payload"
	ReasonNaturalKeyTaken         ReasonCode = "natural_key_taken"
	ReasonDuplicateEmail          ReasonCode = "duplicate_email"
	ReasonContactUnnamed          ReasonCode = "contact_unnamed"
	ReasonContactEmailHeld        ReasonCode = "contact_email_held"
	ReasonCompanyNameEmpty        ReasonCode = "company_name_empty"
	ReasonDomainHeld              ReasonCode = "domain_held"
	ReasonLinkedinHeld            ReasonCode = "linkedin_held"
	ReasonRecordArchived          ReasonCode = "record_archived"
	ReasonCannotCreate            ReasonCode = "cannot_create"
	ReasonCannotEdit              ReasonCode = "cannot_edit"
	ReasonOwnerRefused            ReasonCode = "owner_refused"
	ReasonOwnerUnknown            ReasonCode = "owner_unknown"
	ReasonTagNew                  ReasonCode = "tag_new"
	ReasonTagCreated              ReasonCode = "tag_created"
	ReasonCompanyDuplicateFound   ReasonCode = "company_duplicate_found"
	ReasonContactDuplicateFound   ReasonCode = "contact_duplicate_found"
	ReasonCompanyDuplicateSkipped ReasonCode = "company_duplicate_skipped"
	ReasonContactDuplicateSkipped ReasonCode = "contact_duplicate_skipped"
	ReasonLinkNotFound            ReasonCode = "link_not_found"
	ReasonLinkNotFoundDropped     ReasonCode = "link_not_found_dropped"
	ReasonLinkAmbiguous           ReasonCode = "link_ambiguous"
	ReasonLinkAmbiguousDropped    ReasonCode = "link_ambiguous_dropped"
	ReasonIdOnNonCompany          ReasonCode = "id_on_non_company"
	ReasonIdNotACompanyId         ReasonCode = "id_not_a_company_id"
	ReasonIdNotVisible            ReasonCode = "id_not_visible"
	ReasonRowKeyRepeated          ReasonCode = "row_key_repeated"
	ReasonRowKeyEmpty             ReasonCode = "row_key_empty"
	ReasonFieldInactive           ReasonCode = "field_inactive"
	ReasonValueUnreadable         ReasonCode = "value_unreadable"
	ReasonPhoneNeedsCountry       ReasonCode = "phone_needs_country"
	ReasonValueRefused            ReasonCode = "value_refused"
	ReasonOptionNotAllowed        ReasonCode = "option_not_allowed"
	ReasonOptionNotInList         ReasonCode = "option_not_in_list"
	ReasonOptionsNeedSemicolons   ReasonCode = "options_need_semicolons"
	ReasonCurrencyUnset           ReasonCode = "currency_unset"
	ReasonNotAnAmount             ReasonCode = "not_an_amount"
	ReasonNotYesNo                ReasonCode = "not_yes_no"
	ReasonNotANumber              ReasonCode = "not_a_number"
	ReasonNotADate                ReasonCode = "not_a_date"
	ReasonCannotWriteType         ReasonCode = "cannot_write_type"
	ReasonNotAnEmail              ReasonCode = "not_an_email"
	ReasonNotADomain              ReasonCode = "not_a_domain"
	ReasonDescriptionTooLong      ReasonCode = "description_too_long"
	ReasonSizeBandInvalid         ReasonCode = "size_band_invalid"
	ReasonTagNameInvalid          ReasonCode = "tag_name_invalid"
	ReasonAuthorTooLong           ReasonCode = "author_too_long"
)

// reasonEntry is one code: the names its sentence fills in, and the sentence per language.
type reasonEntry struct {
	params []string
	text   map[string]string
}

// droppedTwin names the code a link problem takes once the run dropped the link
// instead of skipping the row.
var droppedTwin = map[ReasonCode]ReasonCode{
	ReasonLinkNotFound:  ReasonLinkNotFoundDropped,
	ReasonLinkAmbiguous: ReasonLinkAmbiguousDropped,
}

var reasonCatalog = map[ReasonCode]reasonEntry{
	ReasonEmptyPayload: {params: nil, text: map[string]string{
		LangEnglish:    "empty_payload",
		LangGerman:     "Die Zeile enthält keine Felder.",
		LangVietnamese: "Hàng không có trường nào.",
	}},
	ReasonNaturalKeyTaken: {params: nil, text: map[string]string{
		LangEnglish:    "natural_key_already_taken",
		LangGerman:     "Ein Datensatz mit diesem Schlüssel existiert bereits.",
		LangVietnamese: "Đã có một bản ghi dùng khóa này.",
	}},
	ReasonDuplicateEmail: {params: nil, text: map[string]string{
		LangEnglish:    "duplicate_email",
		LangGerman:     "Diese E-Mail-Adresse gehört bereits einem anderen Kontakt.",
		LangVietnamese: "Địa chỉ email này đã thuộc về một liên hệ khác.",
	}},
	ReasonContactUnnamed: {params: nil, text: map[string]string{
		LangEnglish:    "the row carries neither a name nor an email, so it names no contact",
		LangGerman:     "Die Zeile enthält weder einen Namen noch eine E-Mail-Adresse und benennt daher keinen Kontakt.",
		LangVietnamese: "Hàng không có tên lẫn email nên không xác định được liên hệ nào.",
	}},
	ReasonContactEmailHeld: {params: nil, text: map[string]string{
		LangEnglish:    "this email address is already held in the CRM, so the row cannot create a second contact under it",
		LangGerman:     "Diese E-Mail-Adresse ist im CRM bereits vergeben, daher kann die Zeile keinen zweiten Kontakt damit anlegen.",
		LangVietnamese: "Địa chỉ email này đã có trong CRM nên hàng không thể tạo liên hệ thứ hai với nó.",
	}},
	ReasonCompanyNameEmpty: {params: nil, text: map[string]string{
		LangEnglish:    "the mapped display_name is empty, so the row names no company",
		LangGerman:     "Der zugeordnete Anzeigename ist leer, daher benennt die Zeile kein Unternehmen.",
		LangVietnamese: "Tên hiển thị đã ánh xạ để trống nên hàng không xác định được công ty nào.",
	}},
	ReasonDomainHeld: {params: nil, text: map[string]string{
		LangEnglish:    "this domain is already held by another company in the CRM, so the row cannot create a second company under it",
		LangGerman:     "Diese Domain gehört im CRM bereits einem anderen Unternehmen, daher kann die Zeile kein zweites Unternehmen damit anlegen.",
		LangVietnamese: "Tên miền này đã thuộc về một công ty khác trong CRM nên hàng không thể tạo công ty thứ hai với nó.",
	}},
	ReasonLinkedinHeld: {params: nil, text: map[string]string{
		LangEnglish:    "this LinkedIn URL is already held by another record in the CRM, so the row cannot claim it",
		LangGerman:     "Diese LinkedIn-URL gehört im CRM bereits einem anderen Datensatz, daher kann die Zeile sie nicht beanspruchen.",
		LangVietnamese: "URL LinkedIn này đã thuộc về một bản ghi khác trong CRM nên hàng không thể dùng nó.",
	}},
	ReasonRecordArchived: {params: nil, text: map[string]string{
		LangEnglish:    "this row's record was archived, so it is not updated; restore the record to update it",
		LangGerman:     "Der Datensatz dieser Zeile wurde archiviert und wird daher nicht aktualisiert. Stellen Sie ihn wieder her, um ihn zu aktualisieren.",
		LangVietnamese: "Bản ghi của hàng này đã được lưu trữ nên không được cập nhật. Hãy khôi phục bản ghi để cập nhật.",
	}},
	ReasonCannotCreate: {params: []string{"plural"}, text: map[string]string{
		LangEnglish:    "You cannot create {plural}, so the {plural} file is skipped",
		LangGerman:     "Sie dürfen keine Datensätze vom Typ {plural} anlegen, daher wird die Datei {plural} übersprungen.",
		LangVietnamese: "Bạn không có quyền tạo {plural} nên tệp {plural} bị bỏ qua.",
	}},
	ReasonCannotEdit: {params: []string{"plural"}, text: map[string]string{
		LangEnglish:    "You cannot edit {plural}, so a row that changes one already in the CRM is skipped",
		LangGerman:     "Sie dürfen {plural} nicht bearbeiten, daher wird eine Zeile übersprungen, die einen vorhandenen Datensatz ändern würde.",
		LangVietnamese: "Bạn không có quyền sửa {plural} nên hàng làm thay đổi bản ghi đã có trong CRM bị bỏ qua.",
	}},
	ReasonOwnerRefused: {params: nil, text: map[string]string{
		LangEnglish:    "You cannot make this user an owner, so you become the owner of their rows",
		LangGerman:     "Sie dürfen diese Person nicht als Eigentümer eintragen, daher werden Sie Eigentümer ihrer Zeilen.",
		LangVietnamese: "Bạn không thể đặt người dùng này làm chủ sở hữu nên bạn trở thành chủ sở hữu các hàng của họ.",
	}},
	ReasonOwnerUnknown: {params: nil, text: map[string]string{
		LangEnglish:    "no user has this email, so you become the owner",
		LangGerman:     "Kein Benutzer hat diese E-Mail-Adresse, daher werden Sie Eigentümer.",
		LangVietnamese: "Không có người dùng nào có email này nên bạn trở thành chủ sở hữu.",
	}},
	ReasonTagNew: {params: nil, text: map[string]string{
		LangEnglish:    "no tag has this name yet, so the import creates it",
		LangGerman:     "Noch kein Tag hat diesen Namen, daher legt der Import ihn an.",
		LangVietnamese: "Chưa có thẻ nào mang tên này nên quá trình nhập sẽ tạo thẻ.",
	}},
	ReasonTagCreated: {params: nil, text: map[string]string{
		LangEnglish:    "no tag had this name, so the import created it",
		LangGerman:     "Kein Tag hatte diesen Namen, daher hat der Import ihn angelegt.",
		LangVietnamese: "Không có thẻ nào mang tên này nên quá trình nhập đã tạo thẻ.",
	}},
	ReasonCompanyDuplicateFound: {params: nil, text: map[string]string{
		LangEnglish:    "a company of this name is already in the CRM; importing this row creates a second one and files the pair for review",
		LangGerman:     "Ein Unternehmen mit diesem Namen ist bereits im CRM. Der Import dieser Zeile legt ein zweites an und stellt das Paar zur Prüfung bereit.",
		LangVietnamese: "Đã có công ty cùng tên trong CRM. Nhập hàng này sẽ tạo công ty thứ hai và đưa cặp này vào hàng đợi xem xét.",
	}},
	ReasonContactDuplicateFound: {params: nil, text: map[string]string{
		LangEnglish:    "someone matching this row is already in the CRM; a row naming an address already held is refused, and a near match creates and files the pair for review",
		LangGerman:     "Jemand, der zu dieser Zeile passt, ist bereits im CRM. Eine Zeile mit einer bereits vergebenen Adresse wird abgelehnt, ein ähnlicher Treffer legt an und stellt das Paar zur Prüfung bereit.",
		LangVietnamese: "Đã có người khớp với hàng này trong CRM. Hàng có địa chỉ đã tồn tại bị từ chối, còn hàng gần giống sẽ được tạo và đưa cặp này vào hàng đợi xem xét.",
	}},
	ReasonCompanyDuplicateSkipped: {params: nil, text: map[string]string{
		LangEnglish:    "a company of this name is already in the CRM, and this run was asked to skip duplicates",
		LangGerman:     "Ein Unternehmen mit diesem Namen ist bereits im CRM, und dieser Lauf soll Duplikate überspringen.",
		LangVietnamese: "Đã có công ty cùng tên trong CRM và lần nhập này được yêu cầu bỏ qua bản trùng.",
	}},
	ReasonContactDuplicateSkipped: {params: nil, text: map[string]string{
		LangEnglish:    "someone matching this row is already in the CRM, and this run was asked to skip duplicates",
		LangGerman:     "Jemand, der zu dieser Zeile passt, ist bereits im CRM, und dieser Lauf soll Duplikate überspringen.",
		LangVietnamese: "Đã có người khớp với hàng này trong CRM và lần nhập này được yêu cầu bỏ qua bản trùng.",
	}},
	ReasonLinkNotFound: {params: []string{"ref"}, text: map[string]string{
		LangEnglish:    "no company with {ref:q} in this upload, in an earlier import, or in Margince",
		LangGerman:     "Kein Unternehmen mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince.",
		LangVietnamese: "Không có công ty nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince.",
	}},
	ReasonLinkNotFoundDropped: {params: []string{"ref"}, text: map[string]string{
		LangEnglish:    "no company with {ref:q} in this upload, in an earlier import, or in Margince; the row was imported without the link",
		LangGerman:     "Kein Unternehmen mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince. Die Zeile wurde ohne Verknüpfung importiert.",
		LangVietnamese: "Không có công ty nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonLinkAmbiguous: {params: []string{"name"}, text: map[string]string{
		LangEnglish:    "more than one company is named {name:q}, so which one employs this contact is a question only a human can answer",
		LangGerman:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, welches diesen Kontakt beschäftigt.",
		LangVietnamese: "Có nhiều hơn một công ty tên {name:q} nên chỉ con người mới quyết định được công ty nào thuê liên hệ này.",
	}},
	ReasonLinkAmbiguousDropped: {params: []string{"name"}, text: map[string]string{
		LangEnglish:    "more than one company is named {name:q}, so which one employs this contact is a question only a human can answer; the row was imported without the link",
		LangGerman:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, welches diesen Kontakt beschäftigt. Die Zeile wurde ohne Verknüpfung importiert.",
		LangVietnamese: "Có nhiều hơn một công ty tên {name:q} nên chỉ con người mới quyết định được công ty nào thuê liên hệ này. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonIdOnNonCompany: {params: []string{"column", "object"}, text: map[string]string{
		LangEnglish:    "only a company row can name an {column:q}; a {object} row is identified by its own key",
		LangGerman:     "Nur eine Unternehmenszeile kann {column:q} angeben; eine Zeile vom Typ {object} wird über ihren eigenen Schlüssel erkannt.",
		LangVietnamese: "Chỉ hàng công ty mới có thể nêu {column:q}; hàng {object} được nhận diện bằng khóa riêng.",
	}},
	ReasonIdNotACompanyId: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not a company id; export the companies to get theirs, or leave the column empty to import this row as a new company",
		LangGerman:     "{value:q} ist keine Unternehmens-ID. Exportieren Sie die Unternehmen, um ihre IDs zu erhalten, oder lassen Sie die Spalte leer, um die Zeile als neues Unternehmen zu importieren.",
		LangVietnamese: "{value:q} không phải mã công ty. Hãy xuất danh sách công ty để lấy mã, hoặc để trống cột để nhập hàng này làm công ty mới.",
	}},
	ReasonIdNotVisible: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "no company you can see has the id {value:q}; it may have been archived, merged away, or belong to a workspace this file did not come from",
		LangGerman:     "Kein Unternehmen, das Sie sehen können, hat die ID {value:q}. Es kann archiviert oder zusammengeführt worden sein oder zu einem Arbeitsbereich gehören, aus dem diese Datei nicht stammt.",
		LangVietnamese: "Không có công ty nào bạn nhìn thấy có mã {value:q}. Có thể nó đã được lưu trữ, gộp, hoặc thuộc về một không gian làm việc mà tệp này không đến từ đó.",
	}},
	ReasonRowKeyRepeated: {params: []string{"column", "value", "line"}, text: map[string]string{
		LangEnglish:    "the {column:q} value {value:q} is already used by line {line}; each row needs its own, because it is what a re-import matches on and what an undo finds this row by",
		LangGerman:     "Der Wert {value:q} der Spalte {column:q} wird bereits in Zeile {line} verwendet. Jede Zeile braucht einen eigenen, weil ein erneuter Import und das Rückgängigmachen die Zeile daran erkennen.",
		LangVietnamese: "Giá trị {value:q} của cột {column:q} đã được dùng ở dòng {line}. Mỗi hàng cần một giá trị riêng vì nhập lại và hoàn tác nhận ra hàng theo giá trị đó.",
	}},
	ReasonRowKeyEmpty: {params: []string{"column"}, text: map[string]string{
		LangEnglish:    "the {column:q} column is empty, so this row cannot be identified for re-import or undo",
		LangGerman:     "Die Spalte {column:q} ist leer, daher kann diese Zeile für einen erneuten Import oder das Rückgängigmachen nicht erkannt werden.",
		LangVietnamese: "Cột {column:q} để trống nên không nhận diện được hàng này khi nhập lại hoặc hoàn tác.",
	}},
	ReasonFieldInactive: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} cannot be written: this field is no longer active",
		LangGerman:     "{value:q} kann nicht geschrieben werden: Dieses Feld ist nicht mehr aktiv.",
		LangVietnamese: "Không thể ghi {value:q}: trường này không còn hoạt động.",
	}},
	ReasonValueUnreadable: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} cannot be read",
		LangGerman:     "{value:q} kann nicht gelesen werden.",
		LangVietnamese: "Không đọc được {value:q}.",
	}},
	ReasonPhoneNeedsCountry: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} needs its country code, like +49 30 1234567",
		LangGerman:     "{value:q} braucht die Landesvorwahl, zum Beispiel +49 30 1234567.",
		LangVietnamese: "{value:q} cần mã quốc gia, ví dụ +49 30 1234567.",
	}},
	ReasonValueRefused: {params: []string{"value", "detail"}, text: map[string]string{
		LangEnglish:    "{value:q}: {detail}",
		LangGerman:     "{value:q}: {detail}",
		LangVietnamese: "{value:q}: {detail}",
	}},
	ReasonOptionNotAllowed: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not one of this field's allowed values",
		LangGerman:     "{value:q} gehört nicht zu den erlaubten Werten dieses Feldes.",
		LangVietnamese: "{value:q} không thuộc các giá trị được phép của trường này.",
	}},
	ReasonOptionNotInList: {params: []string{"value", "options"}, text: map[string]string{
		LangEnglish:    "{value:q} is not one of the allowed values: {options}",
		LangGerman:     "{value:q} gehört nicht zu den erlaubten Werten: {options}",
		LangVietnamese: "{value:q} không thuộc các giá trị được phép: {options}",
	}},
	ReasonOptionsNeedSemicolons: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q}: separate several values with semicolons",
		LangGerman:     "{value:q}: Trennen Sie mehrere Werte mit Semikolons.",
		LangVietnamese: "{value:q}: hãy phân tách nhiều giá trị bằng dấu chấm phẩy.",
	}},
	ReasonCurrencyUnset: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} cannot be read: this currency field has no currency set",
		LangGerman:     "{value:q} kann nicht gelesen werden: Für dieses Währungsfeld ist keine Währung eingestellt.",
		LangVietnamese: "Không đọc được {value:q}: trường tiền tệ này chưa đặt đơn vị tiền.",
	}},
	ReasonNotAnAmount: {params: []string{"value", "currency"}, text: map[string]string{
		LangEnglish:    "{value:q} is not an amount in {currency}; write it like 12000.50",
		LangGerman:     "{value:q} ist kein Betrag in {currency}. Schreiben Sie ihn so: 12000.50",
		LangVietnamese: "{value:q} không phải số tiền bằng {currency}. Hãy viết như 12000.50.",
	}},
	ReasonNotYesNo: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not yes or no",
		LangGerman:     "{value:q} ist weder Ja noch Nein.",
		LangVietnamese: "{value:q} không phải có hoặc không.",
	}},
	ReasonNotANumber: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not a number",
		LangGerman:     "{value:q} ist keine Zahl.",
		LangVietnamese: "{value:q} không phải là số.",
	}},
	ReasonNotADate: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not a date; write it as YYYY-MM-DD",
		LangGerman:     "{value:q} ist kein Datum. Schreiben Sie es als JJJJ-MM-TT.",
		LangVietnamese: "{value:q} không phải ngày. Hãy viết dạng YYYY-MM-DD.",
	}},
	ReasonCannotWriteType: {params: []string{"value", "type"}, text: map[string]string{
		LangEnglish:    "{value:q} cannot be written to a {type} field",
		LangGerman:     "{value:q} kann nicht in ein Feld vom Typ {type} geschrieben werden.",
		LangVietnamese: "Không thể ghi {value:q} vào trường kiểu {type}.",
	}},
	ReasonNotAnEmail: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not an email address that can be written",
		LangGerman:     "{value:q} ist keine E-Mail-Adresse, die gespeichert werden kann.",
		LangVietnamese: "{value:q} không phải địa chỉ email có thể ghi.",
	}},
	ReasonNotADomain: {params: []string{"value"}, text: map[string]string{
		LangEnglish:    "{value:q} is not a domain that can be written",
		LangGerman:     "{value:q} ist keine Domain, die gespeichert werden kann.",
		LangVietnamese: "{value:q} không phải tên miền có thể ghi.",
	}},
	ReasonDescriptionTooLong: {params: []string{"length", "max"}, text: map[string]string{
		LangEnglish:    "the description is {length} characters; a company description holds at most {max}",
		LangGerman:     "Die Beschreibung hat {length} Zeichen; eine Unternehmensbeschreibung fasst höchstens {max}.",
		LangVietnamese: "Mô tả dài {length} ký tự; mô tả công ty tối đa {max}.",
	}},
	ReasonSizeBandInvalid: {params: []string{"value", "allowed"}, text: map[string]string{
		LangEnglish:    "size_band {value:q} is not one of {allowed}",
		LangGerman:     "Die Größenklasse {value:q} gehört nicht zu: {allowed}",
		LangVietnamese: "Nhóm quy mô {value:q} không thuộc: {allowed}",
	}},
	ReasonTagNameInvalid: {params: []string{"name", "detail"}, text: map[string]string{
		LangEnglish:    "the tag {name:q} cannot be created: its name {detail}",
		LangGerman:     "Der Tag {name:q} kann nicht angelegt werden: Sein Name {detail}",
		LangVietnamese: "Không thể tạo thẻ {name:q}: tên của nó {detail}",
	}},
	ReasonAuthorTooLong: {params: []string{"max"}, text: map[string]string{
		LangEnglish:    "author is longer than {max} characters",
		LangGerman:     "Der Autor ist länger als {max} Zeichen.",
		LangVietnamese: "Tác giả dài hơn {max} ký tự.",
	}},
}
