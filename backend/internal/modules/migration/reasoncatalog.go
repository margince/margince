// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// The codes a report carries and their sentences in each language. The English
// sentence is also what a reader that knows no code is shown, so it is the
// wording the report always had.

// The codes. The contract's ImportReasonCode enum is this list.
const (
	ReasonEmptyPayload               ReasonCode = "empty_payload"
	ReasonNaturalKeyTaken            ReasonCode = "natural_key_taken"
	ReasonDuplicateEmail             ReasonCode = "duplicate_email"
	ReasonContactUnnamed             ReasonCode = "contact_unnamed"
	ReasonContactEmailHeld           ReasonCode = "contact_email_held"
	ReasonAddressErased              ReasonCode = "address_erased"
	ReasonCompanyNameEmpty           ReasonCode = "company_name_empty"
	ReasonDomainHeld                 ReasonCode = "domain_held"
	ReasonLinkedinHeld               ReasonCode = "linkedin_held"
	ReasonRecordArchived             ReasonCode = "record_archived"
	ReasonCannotCreate               ReasonCode = "cannot_create"
	ReasonCannotEdit                 ReasonCode = "cannot_edit"
	ReasonOwnerRefused               ReasonCode = "owner_refused"
	ReasonOwnerUnknown               ReasonCode = "owner_unknown"
	ReasonTagNew                     ReasonCode = "tag_new"
	ReasonTagCreated                 ReasonCode = "tag_created"
	ReasonCompanyDuplicateFound      ReasonCode = "company_duplicate_found"
	ReasonContactDuplicateFound      ReasonCode = "contact_duplicate_found"
	ReasonCompanyDuplicateSkipped    ReasonCode = "company_duplicate_skipped"
	ReasonContactDuplicateSkipped    ReasonCode = "contact_duplicate_skipped"
	ReasonLinkNotFound               ReasonCode = "link_not_found"
	ReasonLinkNotFoundDropped        ReasonCode = "link_not_found_dropped"
	ReasonLinkAmbiguous              ReasonCode = "link_ambiguous"
	ReasonLinkAmbiguousDropped       ReasonCode = "link_ambiguous_dropped"
	ReasonIDOnNonCompany             ReasonCode = "id_on_non_company"
	ReasonIDNotACompanyID            ReasonCode = "id_not_a_company_id"
	ReasonIDNotVisible               ReasonCode = "id_not_visible"
	ReasonRefTakenByAnotherCompany   ReasonCode = "ref_taken_by_another_company"
	ReasonRowKeyRepeated             ReasonCode = "row_key_repeated"
	ReasonRowKeyEmpty                ReasonCode = "row_key_empty"
	ReasonFieldInactive              ReasonCode = "field_inactive"
	ReasonValueUnreadable            ReasonCode = "value_unreadable"
	ReasonPhoneNeedsCountry          ReasonCode = "phone_needs_country"
	ReasonAddressNotSplit            ReasonCode = "address_not_split"
	ReasonValueRefused               ReasonCode = "value_refused"
	ReasonOptionNotAllowed           ReasonCode = "option_not_allowed"
	ReasonOptionNotInList            ReasonCode = "option_not_in_list"
	ReasonOptionsNeedSemicolons      ReasonCode = "options_need_semicolons"
	ReasonCurrencyUnset              ReasonCode = "currency_unset"
	ReasonNotAnAmount                ReasonCode = "not_an_amount"
	ReasonNotYesNo                   ReasonCode = "not_yes_no"
	ReasonNotANumber                 ReasonCode = "not_a_number"
	ReasonNotADate                   ReasonCode = "not_a_date"
	ReasonCannotWriteType            ReasonCode = "cannot_write_type"
	ReasonNotAnEmail                 ReasonCode = "not_an_email"
	ReasonEmailNamesNoContact        ReasonCode = "email_names_no_contact"
	ReasonNotADomain                 ReasonCode = "not_a_domain"
	ReasonDescriptionTooLong         ReasonCode = "description_too_long"
	ReasonSizeBandInvalid            ReasonCode = "size_band_invalid"
	ReasonTagNameInvalid             ReasonCode = "tag_name_invalid"
	ReasonEmployerColumnEmpty        ReasonCode = "employer_column_empty"
	ReasonEmployerNotFound           ReasonCode = "employer_not_found"
	ReasonCannotEditAndTag           ReasonCode = "cannot_edit_and_tag"
	ReasonCannotReadTags             ReasonCode = "cannot_read_tags"
	ReasonCannotCreateTags           ReasonCode = "cannot_create_tags"
	ReasonEmployerColumnEmptyDropped ReasonCode = "employer_column_empty_dropped"
	ReasonEmployerNotFoundDropped    ReasonCode = "employer_not_found_dropped"
	ReasonAuthorTooLong              ReasonCode = "author_too_long"
	ReasonLinkRowNotImported         ReasonCode = "link_row_not_imported"
	ReasonLinkContactNotImported     ReasonCode = "link_contact_not_imported"
	ReasonLinkEdgeUnsupported        ReasonCode = "link_edge_unsupported"
	ReasonLeadStatusUnknown          ReasonCode = "lead_status_unknown"
	ReasonLeadStatusNotImportable    ReasonCode = "lead_status_not_importable"
	ReasonUndoGrantLost              ReasonCode = "undo_grant_lost"
	ReasonUndoNotVisible             ReasonCode = "undo_not_visible"
	ReasonUndoRuleProtects           ReasonCode = "undo_rule_protects"
	ReasonUndoFailed                 ReasonCode = "undo_failed"
)

// reasonEntry is one code: the names its sentence fills in, and the sentence per language.
type reasonEntry struct {
	params []string
	text   map[textlang.Lang]string
}

// droppedTwin names the code a link problem takes once the run dropped the link
// instead of skipping the row.
var droppedTwin = map[ReasonCode]ReasonCode{
	ReasonLinkNotFound:          ReasonLinkNotFoundDropped,
	ReasonContactNotFound:       ReasonContactNotFoundDropped,
	ReasonLinkAmbiguous:         ReasonLinkAmbiguousDropped,
	ReasonEmployerColumnEmpty:   ReasonEmployerColumnEmptyDropped,
	ReasonEmployerNotFound:      ReasonEmployerNotFoundDropped,
	ReasonLeadNotFound:          ReasonLeadNotFoundDropped,
	ReasonDealNotFound:          ReasonDealNotFoundDropped,
	ReasonLinkRowSkipped:        ReasonLinkRowSkippedDropped,
	ReasonActivityLinkAmbiguous: ReasonActivityLinkAmbiguousDropped,
}

// Parameter names and a text three reasons share.
const (
	paramPlural = "plural"
	paramRef    = "ref"
	paramName   = "name"
	paramColumn = "column"
	paramValue  = "value"

	textValueDetail = "{value:q}: {detail}"
)

var reasonCatalog = withUndoReasons(withActivityReasons(withDealReasons(map[ReasonCode]reasonEntry{
	ReasonEmptyPayload: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The row has no fields.",
		textlang.German:     "Die Zeile enthält keine Felder.",
		textlang.Vietnamese: "Hàng không có trường nào.",
	}},
	ReasonNaturalKeyTaken: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A record with this key already exists.",
		textlang.German:     "Ein Datensatz mit diesem Schlüssel existiert bereits.",
		textlang.Vietnamese: "Đã có một bản ghi dùng khóa này.",
	}},
	ReasonDuplicateEmail: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This email address already belongs to another contact.",
		textlang.German:     "Diese E-Mail-Adresse gehört bereits einem anderen Kontakt.",
		textlang.Vietnamese: "Địa chỉ email này đã thuộc về một liên hệ khác.",
	}},
	ReasonContactUnnamed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The row carries neither a name nor an email, so it names no contact.",
		textlang.German:     "Die Zeile enthält weder einen Namen noch eine E-Mail-Adresse und benennt daher keinen Kontakt.",
		textlang.Vietnamese: "Hàng không có tên lẫn email nên không xác định được liên hệ nào.",
	}},
	ReasonContactEmailHeld: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This email address is already held in the CRM, so the row cannot create a second contact under it.",
		textlang.German:     "Diese E-Mail-Adresse ist im CRM bereits vergeben, daher kann die Zeile keinen zweiten Kontakt damit anlegen.",
		textlang.Vietnamese: "Địa chỉ email này đã có trong CRM nên hàng không thể tạo liên hệ thứ hai với nó.",
	}},
	ReasonAddressErased: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Whoever owns this email address asked to be erased and not to be stored again, so the row was not imported.",
		textlang.German:     "Wer diese E-Mail-Adresse besitzt, hat die Löschung verlangt und darf nicht erneut gespeichert werden, daher wurde die Zeile nicht importiert.",
		textlang.Vietnamese: "Chủ của địa chỉ email này đã yêu cầu xóa và không được lưu lại, nên hàng không được nhập.",
	}},
	ReasonCompanyNameEmpty: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The mapped display_name is empty, so the row names no company.",
		textlang.German:     "Der zugeordnete Anzeigename ist leer, daher benennt die Zeile kein Unternehmen.",
		textlang.Vietnamese: "Tên hiển thị đã ánh xạ để trống nên hàng không xác định được công ty nào.",
	}},
	ReasonDomainHeld: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This domain is already held by another company in the CRM, so the row cannot create a second company under it.",
		textlang.German:     "Diese Domain gehört im CRM bereits einem anderen Unternehmen, daher kann die Zeile kein zweites Unternehmen damit anlegen.",
		textlang.Vietnamese: "Tên miền này đã thuộc về một công ty khác trong CRM nên hàng không thể tạo công ty thứ hai với nó.",
	}},
	ReasonLinkedinHeld: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This LinkedIn URL is already held by another record in the CRM, so the row cannot claim it.",
		textlang.German:     "Diese LinkedIn-URL gehört im CRM bereits einem anderen Datensatz, daher kann die Zeile sie nicht beanspruchen.",
		textlang.Vietnamese: "URL LinkedIn này đã thuộc về một bản ghi khác trong CRM nên hàng không thể dùng nó.",
	}},
	ReasonRecordArchived: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This row’s record was archived, so it is not updated. Restore the record to update it.",
		textlang.German:     "Der Datensatz dieser Zeile wurde archiviert und wird daher nicht aktualisiert. Stellen Sie ihn wieder her, um ihn zu aktualisieren.",
		textlang.Vietnamese: "Bản ghi của hàng này đã được lưu trữ nên không được cập nhật. Hãy khôi phục bản ghi để cập nhật.",
	}},
	ReasonCannotCreate: {params: []string{paramPlural}, text: map[textlang.Lang]string{
		textlang.English:    "You cannot create {plural}, so the {plural} file is skipped.",
		textlang.German:     "Sie dürfen keine Datensätze vom Typ {plural} anlegen, daher wird die Datei {plural} übersprungen.",
		textlang.Vietnamese: "Bạn không có quyền tạo {plural} nên tệp {plural} bị bỏ qua.",
	}},
	ReasonCannotEdit: {params: []string{paramPlural}, text: map[textlang.Lang]string{
		textlang.English:    "You cannot edit {plural}, so a row that changes one already in the CRM is skipped.",
		textlang.German:     "Sie dürfen {plural} nicht bearbeiten, daher wird eine Zeile übersprungen, die einen vorhandenen Datensatz ändern würde.",
		textlang.Vietnamese: "Bạn không có quyền sửa {plural} nên hàng làm thay đổi bản ghi đã có trong CRM bị bỏ qua.",
	}},
	ReasonOwnerRefused: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot make this user an owner, so you become the owner of their rows.",
		textlang.German:     "Sie dürfen dieses Mitglied nicht als verantwortlich eintragen, daher sind Sie für dessen Zeilen verantwortlich.",
		textlang.Vietnamese: "Bạn không thể đặt người dùng này làm người chịu trách nhiệm nên bạn trở thành người chịu trách nhiệm các hàng của họ.",
	}},
	ReasonOwnerUnknown: {params: []string{"rows"}, text: map[textlang.Lang]string{
		textlang.English:    "No user has this email, so you become the owner; a lead stays unassigned. Rows naming it: {rows}.",
		textlang.German:     "Kein Mitglied hat diese E-Mail-Adresse, daher sind Sie verantwortlich; ein Lead bleibt nicht zugewiesen. Betroffene Zeilen: {rows}.",
		textlang.Vietnamese: "Không có người dùng nào có email này nên bạn trở thành người chịu trách nhiệm; lead vẫn chưa giao. Số hàng nêu email này: {rows}.",
	}},
	ReasonTagNew: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "No tag has this name yet, so the import creates it.",
		textlang.German:     "Noch kein Tag hat diesen Namen, daher legt der Import ihn an.",
		textlang.Vietnamese: "Chưa có thẻ nào mang tên này nên quá trình nhập sẽ tạo thẻ.",
	}},
	ReasonTagCreated: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "No tag had this name, so the import created it.",
		textlang.German:     "Kein Tag hatte diesen Namen, daher hat der Import ihn angelegt.",
		textlang.Vietnamese: "Không có thẻ nào mang tên này nên quá trình nhập đã tạo thẻ.",
	}},
	ReasonCompanyDuplicateFound: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A company of this name is already in the CRM. Importing this row creates a second one and files the pair for review.",
		textlang.German:     "Ein Unternehmen mit diesem Namen ist bereits im CRM. Der Import dieser Zeile legt ein zweites an und stellt das Paar zur Prüfung bereit.",
		textlang.Vietnamese: "Đã có công ty cùng tên trong CRM. Nhập hàng này sẽ tạo công ty thứ hai và đưa cặp này vào hàng đợi xem xét.",
	}},
	ReasonContactDuplicateFound: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Someone matching this row is already in the CRM. A row naming an address already held is refused, and a near match creates and files the pair for review.",
		textlang.German:     "Jemand, der zu dieser Zeile passt, ist bereits im CRM. Eine Zeile mit einer bereits vergebenen Adresse wird abgelehnt, ein ähnlicher Treffer legt an und stellt das Paar zur Prüfung bereit.",
		textlang.Vietnamese: "Đã có người khớp với hàng này trong CRM. Hàng có địa chỉ đã tồn tại bị từ chối, còn hàng gần giống sẽ được tạo và đưa cặp này vào hàng đợi xem xét.",
	}},
	ReasonCompanyDuplicateSkipped: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A company of this name is already in the CRM, and this run was asked to skip duplicates.",
		textlang.German:     "Ein Unternehmen mit diesem Namen ist bereits im CRM, und dieser Lauf soll Duplikate überspringen.",
		textlang.Vietnamese: "Đã có công ty cùng tên trong CRM và lần nhập này được yêu cầu bỏ qua bản trùng.",
	}},
	ReasonContactDuplicateSkipped: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Someone matching this row is already in the CRM, and this run was asked to skip duplicates.",
		textlang.German:     "Jemand, der zu dieser Zeile passt, ist bereits im CRM, und dieser Lauf soll Duplikate überspringen.",
		textlang.Vietnamese: "Đã có người khớp với hàng này trong CRM và lần nhập này được yêu cầu bỏ qua bản trùng.",
	}},
	ReasonLinkNotFound: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No company with “{ref}” in this upload, in an earlier import, or in Margince.",
		textlang.German:     "Kein Unternehmen mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince.",
		textlang.Vietnamese: "Không có công ty nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince.",
	}},
	ReasonLinkNotFoundDropped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No company with “{ref}” in this upload, in an earlier import, or in Margince. The row was imported without the link.",
		textlang.German:     "Kein Unternehmen mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Không có công ty nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonLinkAmbiguous: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "More than one company is named “{name}”, so which one employs this contact is a question only a human can answer.",
		textlang.German:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, welches diesen Kontakt beschäftigt.",
		textlang.Vietnamese: "Có nhiều hơn một công ty tên {name:q} nên chỉ con người mới quyết định được công ty nào thuê liên hệ này.",
	}},
	ReasonLinkAmbiguousDropped: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "More than one company is named “{name}”, so which one employs this contact is a question only a human can answer. The row was imported without the link.",
		textlang.German:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, welches diesen Kontakt beschäftigt. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Có nhiều hơn một công ty tên {name:q} nên chỉ con người mới quyết định được công ty nào thuê liên hệ này. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonIDOnNonCompany: {params: []string{paramColumn, "object"}, text: map[textlang.Lang]string{
		textlang.English:    "Only a company row can name an “{column}”. A {object} row is identified by its own key.",
		textlang.German:     "Nur eine Unternehmenszeile kann {column:q} angeben; eine Zeile vom Typ {object} wird über ihren eigenen Schlüssel erkannt.",
		textlang.Vietnamese: "Chỉ hàng công ty mới có thể nêu {column:q}; hàng {object} được nhận diện bằng khóa riêng.",
	}},
	ReasonIDNotACompanyID: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a company id. Export the companies to get theirs, or leave the column empty to import this row as a new company.",
		textlang.German:     "{value:q} ist keine Unternehmens-ID. Exportieren Sie die Unternehmen, um ihre IDs zu erhalten, oder lassen Sie die Spalte leer, um die Zeile als neues Unternehmen zu importieren.",
		textlang.Vietnamese: "{value:q} không phải mã công ty. Hãy xuất danh sách công ty để lấy mã, hoặc để trống cột để nhập hàng này làm công ty mới.",
	}},
	ReasonIDNotVisible: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "No company you can see has the id “{value}”. It may have been archived or merged away, or come from a different system.",
		textlang.German:     "Kein Unternehmen, das Sie sehen können, hat die ID {value:q}. Es kann archiviert oder zusammengeführt worden sein oder zu einem Arbeitsbereich gehören, aus dem diese Datei nicht stammt.",
		textlang.Vietnamese: "Không có công ty nào bạn nhìn thấy có mã {value:q}. Có thể nó đã được lưu trữ, gộp, hoặc thuộc về một không gian làm việc mà tệp này không đến từ đó.",
	}},
	ReasonRefTakenByAnotherCompany: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "The reference “{ref}” already stands for another company from an earlier import, so this row is not applied. Use a new reference for this company or fix the id column.",
		textlang.German:     "Die Referenz {ref:q} steht bereits für ein anderes Unternehmen aus einem früheren Import, daher wird diese Zeile nicht übernommen. Verwenden Sie eine neue Referenz für dieses Unternehmen oder korrigieren Sie die ID-Spalte.",
		textlang.Vietnamese: "Tham chiếu {ref:q} đã đại diện cho một công ty khác từ lần nhập trước nên hàng này không được áp dụng. Hãy dùng tham chiếu mới cho công ty này hoặc sửa cột mã.",
	}},
	ReasonRowKeyRepeated: {params: []string{paramColumn, paramValue, "line"}, text: map[textlang.Lang]string{
		textlang.English:    "The “{column}” value “{value}” is already used by line {line}. Each row needs its own, because it is what a re-import matches on and what an undo finds this row by.",
		textlang.German:     "Der Wert {value:q} der Spalte {column:q} wird bereits in Zeile {line} verwendet. Jede Zeile braucht einen eigenen, weil ein erneuter Import und das Rückgängigmachen die Zeile daran erkennen.",
		textlang.Vietnamese: "Giá trị {value:q} của cột {column:q} đã được dùng ở dòng {line}. Mỗi hàng cần một giá trị riêng vì nhập lại và hoàn tác nhận ra hàng theo giá trị đó.",
	}},
	ReasonRowKeyEmpty: {params: []string{paramColumn}, text: map[textlang.Lang]string{
		textlang.English:    "The “{column}” column is empty, so this row cannot be identified for re-import or undo.",
		textlang.German:     "Die Spalte {column:q} ist leer, daher kann diese Zeile für einen erneuten Import oder das Rückgängigmachen nicht erkannt werden.",
		textlang.Vietnamese: "Cột {column:q} để trống nên không nhận diện được hàng này khi nhập lại hoặc hoàn tác.",
	}},
	ReasonFieldInactive: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” cannot be written: this field is no longer active.",
		textlang.German:     "{value:q} kann nicht geschrieben werden: Dieses Feld ist nicht mehr aktiv.",
		textlang.Vietnamese: "Không thể ghi {value:q}: trường này không còn hoạt động.",
	}},
	ReasonValueUnreadable: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” cannot be read.",
		textlang.German:     "{value:q} kann nicht gelesen werden.",
		textlang.Vietnamese: "Không đọc được {value:q}.",
	}},
	ReasonAddressNotSplit: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Address could not be split into street, postal code and city, so it stays in the first address line.",
		textlang.German:     "Die Adresse ließ sich nicht in Straße, Postleitzahl und Ort aufteilen, daher bleibt sie in der ersten Adresszeile.",
		textlang.Vietnamese: "Không tách được địa chỉ thành đường, mã bưu chính và thành phố nên địa chỉ vẫn nằm ở dòng địa chỉ đầu tiên.",
	}},
	ReasonPhoneNeedsCountry: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” needs its country code, like +49 30 1234567.",
		textlang.German:     "{value:q} braucht die Landesvorwahl, zum Beispiel +49 30 1234567.",
		textlang.Vietnamese: "{value:q} cần mã quốc gia, ví dụ +49 30 1234567.",
	}},
	ReasonValueRefused: {params: []string{paramValue, "detail"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” was refused: {detail}",
		textlang.German:     textValueDetail,
		textlang.Vietnamese: textValueDetail,
	}},
	ReasonOptionNotAllowed: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not one of this field’s allowed values.",
		textlang.German:     "{value:q} gehört nicht zu den erlaubten Werten dieses Feldes.",
		textlang.Vietnamese: "{value:q} không thuộc các giá trị được phép của trường này.",
	}},
	ReasonOptionNotInList: {params: []string{paramValue, "options"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not one of the allowed values: {options}.",
		textlang.German:     "{value:q} gehört nicht zu den erlaubten Werten: {options}",
		textlang.Vietnamese: "{value:q} không thuộc các giá trị được phép: {options}",
	}},
	ReasonOptionsNeedSemicolons: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}”: separate several values with semicolons.",
		textlang.German:     "{value:q}: Trennen Sie mehrere Werte mit Semikolons.",
		textlang.Vietnamese: "{value:q}: hãy phân tách nhiều giá trị bằng dấu chấm phẩy.",
	}},
	ReasonCurrencyUnset: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” cannot be read: this currency field has no currency set.",
		textlang.German:     "{value:q} kann nicht gelesen werden: Für dieses Währungsfeld ist keine Währung eingestellt.",
		textlang.Vietnamese: "Không đọc được {value:q}: trường tiền tệ này chưa đặt đơn vị tiền.",
	}},
	ReasonNotAnAmount: {params: []string{paramValue, "currency"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not an amount in {currency}. Write it like 12000.50.",
		textlang.German:     "{value:q} ist kein Betrag in {currency}. Schreiben Sie ihn so: 12000.50",
		textlang.Vietnamese: "{value:q} không phải số tiền bằng {currency}. Hãy viết như 12000.50.",
	}},
	ReasonNotYesNo: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not yes or no.",
		textlang.German:     "{value:q} ist weder Ja noch Nein.",
		textlang.Vietnamese: "{value:q} không phải có hoặc không.",
	}},
	ReasonNotANumber: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a number.",
		textlang.German:     "{value:q} ist keine Zahl.",
		textlang.Vietnamese: "{value:q} không phải là số.",
	}},
	ReasonNotADate: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a date. Write it as YYYY-MM-DD.",
		textlang.German:     "{value:q} ist kein Datum. Schreiben Sie es als JJJJ-MM-TT.",
		textlang.Vietnamese: "{value:q} không phải ngày. Hãy viết dạng YYYY-MM-DD.",
	}},
	ReasonCannotWriteType: {params: []string{paramValue, "type"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” cannot be written to a {type} field.",
		textlang.German:     "{value:q} kann nicht in ein Feld vom Typ {type} geschrieben werden.",
		textlang.Vietnamese: "Không thể ghi {value:q} vào trường kiểu {type}.",
	}},
	ReasonNotAnEmail: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not an email address that can be written.",
		textlang.German:     "{value:q} ist keine E-Mail-Adresse, die gespeichert werden kann.",
		textlang.Vietnamese: "{value:q} không phải địa chỉ email có thể ghi.",
	}},
	ReasonEmailNamesNoContact: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” belongs to a sending system, not a human, so it does not become a contact.",
		textlang.German:     "{value:q} gehört zu einem Versandsystem, nicht zu einem Menschen, und wird darum kein Kontakt.",
		textlang.Vietnamese: "{value:q} thuộc về một hệ thống gửi thư, không phải của một cá nhân, nên không trở thành liên hệ.",
	}},
	ReasonNotADomain: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a domain that can be written.",
		textlang.German:     "{value:q} ist keine Domain, die gespeichert werden kann.",
		textlang.Vietnamese: "{value:q} không phải tên miền có thể ghi.",
	}},
	ReasonDescriptionTooLong: {params: []string{"length", "max"}, text: map[textlang.Lang]string{
		textlang.English:    "The description is {length} characters. A company description holds at most {max}.",
		textlang.German:     "Die Beschreibung hat {length} Zeichen; eine Unternehmensbeschreibung fasst höchstens {max}.",
		textlang.Vietnamese: "Mô tả dài {length} ký tự; mô tả công ty tối đa {max}.",
	}},
	ReasonSizeBandInvalid: {params: []string{paramValue, "allowed"}, text: map[textlang.Lang]string{
		textlang.English:    "Size_band “{value}” is not one of {allowed}.",
		textlang.German:     "Die Größenklasse {value:q} gehört nicht zu: {allowed}",
		textlang.Vietnamese: "Nhóm quy mô {value:q} không thuộc: {allowed}",
	}},
	ReasonTagNameInvalid: {params: []string{paramName, "detail"}, text: map[textlang.Lang]string{
		textlang.English:    "The tag “{name}” cannot be created: its name {detail}.",
		textlang.German:     "Der Tag {name:q} kann nicht angelegt werden: Sein Name {detail}",
		textlang.Vietnamese: "Không thể tạo thẻ {name:q}: tên của nó {detail}",
	}},
	ReasonEmployerColumnEmpty: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The company column is empty, so the row names no employer.",
		textlang.German:     "Die Unternehmensspalte ist leer, daher nennt die Zeile keinen Arbeitgeber.",
		textlang.Vietnamese: "Cột công ty để trống nên hàng không nêu nhà tuyển dụng nào.",
	}},
	ReasonEmployerNotFound: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "No company named “{name}” is in the CRM, so there was nothing to link this contact to.",
		textlang.German:     "Im CRM gibt es kein Unternehmen namens {name:q}, daher gab es nichts, womit dieser Kontakt verknüpft werden konnte.",
		textlang.Vietnamese: "Không có công ty nào tên {name:q} trong CRM nên không có gì để liên kết liên hệ này.",
	}},
	ReasonCannotEditAndTag: {params: []string{paramPlural}, text: map[textlang.Lang]string{
		textlang.English:    "You cannot edit {plural}, so a row that changes one already in the CRM is skipped and no tag is added to one.",
		textlang.German:     "Sie dürfen {plural} nicht bearbeiten, daher wird eine Zeile übersprungen, die einen vorhandenen Datensatz ändern würde, und es wird kein Tag hinzugefügt.",
		textlang.Vietnamese: "Bạn không có quyền sửa {plural} nên hàng làm thay đổi bản ghi đã có trong CRM bị bỏ qua và không thẻ nào được thêm vào.",
	}},
	ReasonCannotReadTags: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot read tags, so the tags column is not imported.",
		textlang.German:     "Sie dürfen Tags nicht lesen, daher wird die Tag-Spalte nicht importiert.",
		textlang.Vietnamese: "Bạn không có quyền đọc thẻ nên cột thẻ không được nhập.",
	}},
	ReasonCannotCreateTags: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot create tags, so a tag new to the CRM is not added.",
		textlang.German:     "Sie dürfen keine Tags anlegen, daher wird ein im CRM neuer Tag nicht hinzugefügt.",
		textlang.Vietnamese: "Bạn không có quyền tạo thẻ nên thẻ mới đối với CRM không được thêm.",
	}},
	ReasonEmployerColumnEmptyDropped: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The company column is empty, so the row names no employer. The row was imported without the link.",
		textlang.German:     "Die Unternehmensspalte ist leer, daher nennt die Zeile keinen Arbeitgeber. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Cột công ty để trống nên hàng không nêu nhà tuyển dụng nào. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonEmployerNotFoundDropped: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "No company named “{name}” is in the CRM, so there was nothing to link this contact to. The row was imported without the link.",
		textlang.German:     "Im CRM gibt es kein Unternehmen namens {name:q}, daher gab es nichts, womit dieser Kontakt verknüpft werden konnte. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Không có công ty nào tên {name:q} trong CRM nên không có gì để liên kết liên hệ này. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonAuthorTooLong: {params: []string{"max"}, text: map[textlang.Lang]string{
		textlang.English:    "Author is longer than {max} characters.",
		textlang.German:     "Der Verfasser ist länger als {max} Zeichen.",
		textlang.Vietnamese: "Tác giả dài hơn {max} ký tự.",
	}},
	ReasonLinkRowNotImported: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "This row will not be imported, so there is nobody to link to {name}.",
		textlang.German:     "Diese Zeile wird nicht importiert, daher gibt es niemanden, der mit {name} verknüpft werden könnte.",
		textlang.Vietnamese: "Hàng này sẽ không được nhập nên không có ai để liên kết với {name}.",
	}},
	ReasonLinkContactNotImported: {params: []string{"from", paramName}, text: map[textlang.Lang]string{
		textlang.English:    "The contact “{from}” was not imported, so there was nobody to link to “{name}”.",
		textlang.German:     "Der Kontakt {from:q} wurde nicht importiert, daher gab es niemanden, der mit {name:q} verknüpft werden konnte.",
		textlang.Vietnamese: "Liên hệ {from:q} chưa được nhập nên không có ai để liên kết với {name:q}.",
	}},
	ReasonLinkEdgeUnsupported: {params: []string{"from", "to"}, text: map[textlang.Lang]string{
		textlang.English:    "A CSV import has no {from} → {to} link, so it was not applied.",
		textlang.German:     "Ein CSV-Import kennt keine Verknüpfung {from}→{to}; sie wurde nicht angewendet.",
		textlang.Vietnamese: "Nhập từ tệp CSV không có liên kết {from}→{to}; liên kết này không được áp dụng.",
	}},
	ReasonLeadStatusUnknown: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a lead status. The statuses are: {allowed}.",
		textlang.German:     "{value:q} ist kein Lead-Status. Es gibt: {allowed}.",
		textlang.Vietnamese: "{value:q} không phải trạng thái lead. Các trạng thái: {allowed}.",
	}},
	ReasonLeadStatusNotImportable: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "A file cannot set the status “{value}”. Use Qualify or Disqualify on the lead. The statuses a file may set are: {allowed}.",
		textlang.German:     "Eine Datei kann den Status {value:q} nicht setzen. Nutzen Sie Qualifizieren oder Disqualifizieren am Lead. Eine Datei darf setzen: {allowed}.",
		textlang.Vietnamese: "Tệp không thể đặt trạng thái {value:q}. Dùng Xác nhận đủ điều kiện hoặc Loại lead trên lead. Trạng thái tệp được đặt: {allowed}.",
	}},
	ReasonUndoGrantLost: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Your permissions no longer cover this record.",
		textlang.German:     "Die Berechtigung des Aufrufers deckt diesen Datensatz nicht mehr ab.",
		textlang.Vietnamese: "Quyền của người thực hiện không còn bao gồm bản ghi này.",
	}},
	ReasonUndoNotVisible: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This record is no longer visible to you.",
		textlang.German:     "Der Datensatz ist für diesen Aufrufer nicht mehr sichtbar.",
		textlang.Vietnamese: "Người thực hiện không còn thấy bản ghi này.",
	}},
	ReasonUndoRuleProtects: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The record refused the undo because a business rule protects it.",
		textlang.German:     "Der Datensatz hat die Rücknahme abgelehnt (eine Geschäftsregel schützt ihn).",
		textlang.Vietnamese: "Bản ghi từ chối việc hoàn tác (một quy tắc nghiệp vụ bảo vệ nó).",
	}},
	ReasonUndoFailed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The record could not be undone.",
		textlang.German:     "Der Datensatz konnte nicht zurückgenommen werden.",
		textlang.Vietnamese: "Không thể hoàn tác bản ghi này.",
	}},
})))
