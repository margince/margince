// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The reasons an activity row is skipped or warned about: its type, timestamp,
// duration, meeting status and direction. They sit apart from the other
// objects' catalog to keep each file under the length cap, and
// withActivityReasons joins the two.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Parameter names the activity reasons share.
const (
	paramAllowed = "allowed"
	paramMax     = "max"
)

// Reason codes for activity rows, each with its reasons text in the catalog below.
const (
	ReasonActivityEmpty                ReasonCode = "activity_empty"
	ReasonActivityTypeUnnamed          ReasonCode = "activity_type_unnamed"
	ReasonActivityTypeUnknown          ReasonCode = "activity_type_unknown"
	ReasonMessageNotImportable         ReasonCode = "message_not_importable"
	ReasonTaskNotImportable            ReasonCode = "task_not_importable"
	ReasonOccurredAtUnnamed            ReasonCode = "occurred_at_unnamed"
	ReasonTimestampNeedsZone           ReasonCode = "timestamp_needs_zone"
	ReasonOccurredAtFuture             ReasonCode = "occurred_at_future"
	ReasonDurationNotValidForKind      ReasonCode = "duration_not_valid_for_kind"
	ReasonDurationOutOfRange           ReasonCode = "duration_out_of_range"
	ReasonMeetingStatusUnknown         ReasonCode = "meeting_status_unknown"
	ReasonMeetingStatusNotForKind      ReasonCode = "meeting_status_not_valid_for_kind"
	ReasonMeetingNotTakenPlace         ReasonCode = "meeting_not_taken_place"
	ReasonDirectionUnknown             ReasonCode = "direction_unknown"
	ReasonActivityUnlinked             ReasonCode = "activity_unlinked"
	ReasonLinkMalformed                ReasonCode = "link_malformed"
	ReasonLinkTypeUnknown              ReasonCode = "link_type_unknown"
	ReasonTooManyLinks                 ReasonCode = "too_many_links"
	ReasonCompanyLinkOnMeeting         ReasonCode = "company_link_on_meeting"
	ReasonLinkNotAttachable            ReasonCode = "link_not_attachable"
	ReasonActivityHeld                 ReasonCode = "activity_held"
	ReasonNoteRepeated                 ReasonCode = "note_repeated"
	ReasonLeadNotFound                 ReasonCode = "lead_not_found"
	ReasonLeadNotFoundDropped          ReasonCode = "lead_not_found_dropped"
	ReasonDealNotFound                 ReasonCode = "deal_not_found"
	ReasonDealNotFoundDropped          ReasonCode = "deal_not_found_dropped"
	ReasonLinkRowSkipped               ReasonCode = "link_row_skipped"
	ReasonLinkRowSkippedDropped        ReasonCode = "link_row_skipped_dropped"
	ReasonActivityLinkAmbiguous        ReasonCode = "activity_link_ambiguous"
	ReasonActivityLinkAmbiguousDropped ReasonCode = "activity_link_ambiguous_dropped"
)

// withActivityReasons adds the activity reasons to the catalog the other
// objects declare, and answers it.
func withActivityReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, activityReasons)
	return catalog
}

var activityReasons = map[ReasonCode]reasonEntry{
	ReasonActivityEmpty: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A note or a call needs a subject or a body. Fill one of them in this row.",
		textlang.German:     "Eine Notiz oder ein Anruf braucht einen Betreff oder einen Text. Fülle in dieser Zeile einen von beiden aus.",
		textlang.Vietnamese: "Ghi chú hoặc cuộc gọi cần có tiêu đề hoặc nội dung. Điền tiêu đề hoặc nội dung vào hàng này.",
	}},
	ReasonActivityTypeUnnamed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Name the activity’s type: note, call, meeting or email.",
		textlang.German:     "Nenne den Typ der Aktivität: note, call, meeting oder email.",
		textlang.Vietnamese: "Hãy nêu loại hoạt động: “note”, “call”, “meeting” hoặc “email”.",
	}},
	ReasonActivityTypeUnknown: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not an activity type. The types are: {allowed}.",
		textlang.German:     "{value:q} ist kein Aktivitätstyp. Es gibt: {allowed}.",
		textlang.Vietnamese: "{value:q} không phải loại hoạt động. Các loại: {allowed}.",
	}},
	ReasonMessageNotImportable: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A message needs a channel, so it cannot be imported from a file. Use a note or an email instead.",
		textlang.German:     "Eine Nachricht braucht einen Kanal und lässt sich daher nicht aus einer Datei importieren. Nehmen Sie stattdessen eine Notiz oder eine E-Mail.",
		textlang.Vietnamese: "Tin nhắn cần một kênh nên không thể nhập từ tệp. Hãy dùng ghi chú hoặc email.",
	}},
	ReasonTaskNotImportable: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A task would land open on your own list, so it is not imported. Use a note instead.",
		textlang.German:     "Eine Aufgabe würde offen auf Ihrer eigenen Liste landen und wird daher nicht importiert. Nehmen Sie stattdessen eine Notiz.",
		textlang.Vietnamese: "Nhiệm vụ sẽ nằm mở trong danh sách của chính bạn nên không được nhập. Hãy dùng ghi chú.",
	}},
	ReasonOccurredAtUnnamed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Give the day the activity happened, as YYYY-MM-DD or with its time zone.",
		textlang.German:     "Gib den Tag an, an dem die Aktivität stattfand, als YYYY-MM-DD oder mit Zeitzone.",
		textlang.Vietnamese: "Hãy nêu ngày hoạt động xảy ra, dạng YYYY-MM-DD hoặc kèm múi giờ.",
	}},
	ReasonTimestampNeedsZone: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” has a time but no time zone. Add the zone, like 2026-03-02T10:00:00+07:00.",
		textlang.German:     "{value:q} hat eine Uhrzeit, aber keine Zeitzone. Ergänze die Zone, etwa 2026-03-02T10:00:00+07:00.",
		textlang.Vietnamese: "{value:q} có giờ nhưng không có múi giờ. Hãy thêm múi giờ, ví dụ 2026-03-02T10:00:00+07:00.",
	}},
	ReasonOccurredAtFuture: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is in the future, and an imported activity must already have happened.",
		textlang.German:     "{value:q} liegt in der Zukunft, und eine importierte Aktivität muss bereits stattgefunden haben.",
		textlang.Vietnamese: "{value:q} ở tương lai, mà hoạt động được nhập phải đã xảy ra.",
	}},
	ReasonDurationNotValidForKind: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Only a call or a meeting has a duration. Remove it from this row.",
		textlang.German:     "Nur ein Anruf oder ein Termin hat eine Dauer. Entferne sie aus dieser Zeile.",
		textlang.Vietnamese: "Chỉ cuộc gọi hoặc cuộc họp mới có thời lượng. Hãy bỏ giá trị này khỏi hàng.",
	}},
	ReasonDurationOutOfRange: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” minutes is outside what an activity can last: zero up to 31 days.",
		textlang.German:     "{value:q} Minuten liegen außerhalb dessen, was eine Aktivität dauern kann: null bis 31 Tage.",
		textlang.Vietnamese: "{value:q} phút nằm ngoài thời lượng một hoạt động có thể có: từ không đến 31 ngày.",
	}},
	ReasonMeetingStatusUnknown: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a meeting status. The statuses are: {allowed}.",
		textlang.German:     "{value:q} ist kein Terminstatus. Es gibt: {allowed}.",
		textlang.Vietnamese: "{value:q} không phải trạng thái cuộc họp. Các trạng thái: {allowed}.",
	}},
	ReasonMeetingStatusNotForKind: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Only a meeting has a meeting status. Remove it from this row.",
		textlang.German:     "Nur ein Termin hat einen Terminstatus. Entferne ihn aus dieser Zeile.",
		textlang.Vietnamese: "Chỉ cuộc họp mới có trạng thái cuộc họp. Hãy bỏ giá trị này khỏi hàng.",
	}},
	ReasonMeetingNotTakenPlace: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This meeting has not taken place yet, so it cannot be held or no_show. Leave the status empty, or use booked or canceled.",
		textlang.German:     "Dieser Termin hat noch nicht stattgefunden und kann daher weder held noch no_show sein. Lass den Status leer oder nimm booked oder canceled.",
		textlang.Vietnamese: "Cuộc họp này chưa diễn ra nên không thể là “held” hoặc “no_show”. Hãy để trống trạng thái hoặc dùng “booked” hay “canceled”.",
	}},
	ReasonDirectionUnknown: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a direction. The directions are: {allowed}.",
		textlang.German:     "{value:q} ist keine Richtung. Es gibt: {allowed}.",
		textlang.Vietnamese: "{value:q} không phải chiều hoạt động. Các chiều: {allowed}.",
	}},
	ReasonActivityUnlinked: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This activity names no record to be filed under. Add a link like deal:D-1001 or contact:jo@acme.com.",
		textlang.German:     "Diese Aktivität nennt keinen Datensatz, unter dem sie abgelegt wird. Ergänze eine Verknüpfung wie deal:D-1001 oder contact:jo@acme.com.",
		textlang.Vietnamese: "Hoạt động này không nêu bản ghi nào để gắn vào. Hãy thêm liên kết như deal:D-1001 hoặc contact:jo@acme.com.",
	}},
	ReasonLinkMalformed: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a link. Write the type, a colon and the key, like deal:D-1001.",
		textlang.German:     "{value:q} ist keine Verknüpfung. Schreibe den Typ, einen Doppelpunkt und den Schlüssel, etwa deal:D-1001.",
		textlang.Vietnamese: "{value:q} không phải liên kết. Hãy viết loại, dấu hai chấm và khóa, ví dụ deal:D-1001.",
	}},
	ReasonLinkTypeUnknown: {params: []string{paramValue, paramAllowed}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a link type. The types are: {allowed}.",
		textlang.German:     "{value:q} ist kein Verknüpfungstyp. Es gibt: {allowed}.",
		textlang.Vietnamese: "{value:q} không phải loại liên kết. Các loại: {allowed}.",
	}},
	ReasonTooManyLinks: {params: []string{paramMax}, text: map[textlang.Lang]string{
		textlang.English:    "An activity can name at most {max} links.",
		textlang.German:     "Eine Aktivität kann höchstens {max} Verknüpfungen nennen.",
		textlang.Vietnamese: "Một hoạt động chỉ được nêu tối đa {max} liên kết.",
	}},
	ReasonCompanyLinkOnMeeting: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A call or a meeting is with a contact, not a company. Link the contact who was there.",
		textlang.German:     "Ein Anruf oder Termin findet mit einem Kontakt statt, nicht mit einem Unternehmen. Verknüpfe den Kontakt, der dabei war.",
		textlang.Vietnamese: "Cuộc gọi hoặc cuộc họp diễn ra với một liên hệ, không phải với công ty. Hãy liên kết liên hệ đã tham gia.",
	}},
	ReasonLinkNotAttachable: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot add activities to one of the records this row links to, so the row is skipped.",
		textlang.German:     "Sie dürfen bei einem der Datensätze, mit denen diese Zeile verknüpft ist, keine Aktivitäten hinzufügen, daher wird die Zeile übersprungen.",
		textlang.Vietnamese: "Bạn không thể thêm hoạt động vào một trong các bản ghi mà hàng này liên kết nên hàng bị bỏ qua.",
	}},
	ReasonActivityHeld: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This activity is held under a retention obligation, so the row cannot change it.",
		textlang.German:     "Diese Aktivität unterliegt einer Aufbewahrungspflicht, daher kann die Zeile sie nicht ändern.",
		textlang.Vietnamese: "Hoạt động này đang bị giữ theo nghĩa vụ lưu trữ nên hàng không thể thay đổi nó.",
	}},
	ReasonNoteRepeated: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This note repeats an earlier row of this import: same records, same text, same time. It is filed once.",
		textlang.German:     "Diese Notiz wiederholt eine frühere Zeile dieses Imports: dieselben Datensätze, derselbe Text, dieselbe Zeit. Abgelegt wird sie nur einmal.",
		textlang.Vietnamese: "Ghi chú này lặp lại một hàng trước đó của lần nhập này: cùng bản ghi, cùng nội dung, cùng thời điểm. Nó chỉ được lưu một lần.",
	}},
	ReasonLeadNotFound: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No lead with “{ref}” in this upload, in an earlier import, or in Margince.",
		textlang.German:     "Kein Lead mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince.",
		textlang.Vietnamese: "Không có lead nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince.",
	}},
	ReasonLeadNotFoundDropped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No lead with “{ref}” in this upload, in an earlier import, or in Margince. The row was imported without the link.",
		textlang.German:     "Kein Lead mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Không có lead nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonDealNotFound: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No deal with “{ref}” in this upload, in an earlier import, or in Margince.",
		textlang.German:     "Kein Deal mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince.",
		textlang.Vietnamese: "Không có deal nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince.",
	}},
	ReasonDealNotFoundDropped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No deal with “{ref}” in this upload, in an earlier import, or in Margince. The row was imported without the link.",
		textlang.German:     "Kein Deal mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Không có deal nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonLinkRowSkipped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "The row “{ref}” was skipped in this import, so there is nothing to link to.",
		textlang.German:     "Die Zeile {ref:q} wurde in diesem Import übersprungen, daher gibt es nichts zu verknüpfen.",
		textlang.Vietnamese: "Hàng {ref:q} đã bị bỏ qua trong lần nhập này nên không có gì để liên kết.",
	}},
	ReasonLinkRowSkippedDropped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "The row “{ref}” was skipped in this import, so there is nothing to link to. The row was imported without the link.",
		textlang.German:     "Die Zeile {ref:q} wurde in diesem Import übersprungen, daher gibt es nichts zu verknüpfen. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Hàng {ref:q} đã bị bỏ qua trong lần nhập này nên không có gì để liên kết. Hàng đã được nhập mà không có liên kết.",
	}},
	ReasonActivityLinkAmbiguous: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "More than one company is named “{name}”, so which one this activity is about is a question only a human can answer.",
		textlang.German:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, um welches es in dieser Aktivität geht.",
		textlang.Vietnamese: "Có nhiều hơn một công ty tên {name:q} nên cần một thành viên quyết định hoạt động này nói về công ty nào.",
	}},
	ReasonActivityLinkAmbiguousDropped: {params: []string{paramName}, text: map[textlang.Lang]string{
		textlang.English:    "More than one company is named “{name}”, so which one this activity is about is a question only a human can answer. The row was imported without the link.",
		textlang.German:     "Mehr als ein Unternehmen heißt {name:q}, daher kann nur ein Mensch entscheiden, um welches es in dieser Aktivität geht. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Có nhiều hơn một công ty tên {name:q} nên cần một thành viên quyết định hoạt động này nói về công ty nào. Hàng đã được nhập mà không có liên kết.",
	}},
}
