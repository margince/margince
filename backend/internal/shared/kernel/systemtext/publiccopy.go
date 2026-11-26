// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var publicDefinitions = map[string]Definition{
	"booking.lets_meet": {Words: langcopy.Phrase{En: "Find a time to meet", De: "Treffen wir uns", Vi: "Hãy gặp nhau"}, Retired: wordings{textlang.English: {"Let's meet"}}},
	"booking.meeting":   {Words: langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"}},
	"booking.proposed":  {Words: langcopy.Phrase{En: "Meeting proposed: %s", De: "Vorgeschlagener Termin: %s", Vi: "Đề xuất cuộc họp: %s"}, Arguments: []string{argumentText}},

	"webform.thanks":          {Words: langcopy.Phrase{En: "Thank you. Your message has been received.", De: "Danke. Die Nachricht ist angekommen.", Vi: "Xin cảm ơn. Đã nhận được tin nhắn."}, Arguments: []string{}},
	"webform.unavailable_now": {Words: langcopy.Phrase{En: "This form could not be sent right now. Try again later.", De: "Das Formular konnte gerade nicht gesendet werden. Bitte später erneut versuchen.", Vi: "Hiện không thể gửi biểu mẫu này. Vui lòng thử lại sau."}, Arguments: []string{}, Retired: wordings{textlang.English: {"This form could not be sent right now. Please try again later."}}},
	"webform.needs_email":     {Words: langcopy.Phrase{En: "This form could not be sent because the email address is missing or not valid. Go back and enter it.", De: "Das Formular konnte nicht gesendet werden, da die E-Mail-Adresse fehlt oder ungültig ist. Bitte zurückgehen und sie eingeben.", Vi: "Không thể gửi biểu mẫu vì địa chỉ email bị thiếu hoặc không hợp lệ. Vui lòng quay lại và nhập địa chỉ email."}, Arguments: []string{}, Retired: wordings{textlang.English: {"This form could not be sent because the email address is missing or not valid. Please go back and enter it."}}},
	"webform.needs_consent":   {Words: langcopy.Phrase{En: "This form could not be sent because the consent box is not ticked. Go back and tick it.", De: "Das Formular konnte nicht gesendet werden, da die Einwilligung nicht angekreuzt ist. Bitte zurückgehen und sie ankreuzen.", Vi: "Không thể gửi biểu mẫu vì ô đồng ý chưa được đánh dấu. Vui lòng quay lại và đánh dấu ô này."}, Arguments: []string{}, Retired: wordings{textlang.English: {"This form could not be sent because the consent box is not ticked. Please go back and tick it."}}},
	"webform.gone":            {Words: langcopy.Phrase{En: "This form is no longer available.", De: "Dieses Formular ist nicht mehr verfügbar.", Vi: "Biểu mẫu này không còn khả dụng."}, Arguments: []string{}},
	"webform.paused":          {Words: langcopy.Phrase{En: "This form is not taking submissions right now.", De: "Dieses Formular nimmt derzeit keine Anfragen an.", Vi: "Biểu mẫu này hiện không nhận lượt gửi mới."}, Arguments: []string{}},
	"webform.too_many":        {Words: langcopy.Phrase{En: "This form receives too many submissions right now. Wait %d seconds and send it again.", De: "Dieses Formular erhält gerade zu viele Anfragen. Bitte %d Sekunden warten und erneut senden.", Vi: "Biểu mẫu này đang nhận quá nhiều lượt gửi. Vui lòng đợi %d giây rồi gửi lại."}, Arguments: []string{argumentCount}},

	"file.unavailable_title":    {Words: langcopy.Phrase{En: "File not available", De: "Datei nicht verfügbar", Vi: "Tệp không khả dụng"}, Arguments: []string{}},
	"file.unavailable_sentence": {Words: langcopy.Phrase{En: "This file was deleted or is no longer accessible.", De: "Diese Datei wurde gelöscht oder ist nicht mehr zugänglich.", Vi: "Tệp này đã bị xóa hoặc không thể truy cập nữa."}, Arguments: []string{}, Retired: wordings{textlang.English: {"This file was deleted or you no longer have access."}}},

	"preview.open_booking":         {Words: langcopy.Phrase{En: "Open booking page", De: "Buchungsseite öffnen", Vi: "Mở trang đặt lịch"}, Arguments: []string{}},
	"preview.page_unavailable":     {Words: langcopy.Phrase{En: "Booking page unavailable", De: "Buchungsseite nicht verfügbar", Vi: "Trang đặt lịch không khả dụng"}, Arguments: []string{}},
	"preview.booking_unavailable":  {Words: langcopy.Phrase{En: "Booking unavailable", De: "Buchung nicht verfügbar", Vi: "Không thể đặt lịch"}, Arguments: []string{}},
	"preview.booking_paused":       {Words: langcopy.Phrase{En: "This booking page is not taking new meetings right now.", De: "Diese Buchungsseite nimmt derzeit keine neuen Termine an.", Vi: "Trang đặt lịch này hiện không nhận cuộc họp mới."}, Arguments: []string{}},
	"preview.book_with":            {Words: langcopy.Phrase{En: "Book a meeting with %s", De: "Termin mit %s buchen", Vi: "Đặt cuộc họp với %s"}, Arguments: []string{argumentText}},
	"preview.book_summary":         {Words: langcopy.Phrase{En: "%s: a %d-minute meeting with %s. Choose a time that suits you.", De: "%s: ein Termin von %d Minuten mit %s. Wähle eine passende Zeit.", Vi: "%s: cuộc họp %d phút với %s. Chọn thời gian phù hợp."}, Arguments: []string{argumentText, argumentCount, argumentText}},
	"preview.book_company_summary": {Words: langcopy.Phrase{En: "%s: a %d-minute meeting with %s of %s. Choose a time that suits you.", De: "%s: ein Termin von %d Minuten mit %s von %s. Wähle eine passende Zeit.", Vi: "%s: cuộc họp %d phút với %s từ %s. Chọn thời gian phù hợp."}, Arguments: []string{argumentText, argumentCount, argumentText, argumentText}},
	"preview.open_record":          {Words: langcopy.Phrase{En: "Open in Margince", De: "In Margince öffnen", Vi: "Mở trong Margince"}, Arguments: []string{}},
	"preview.record_title":         {Words: langcopy.Phrase{En: recordPreviewTitle, De: recordPreviewTitle, Vi: recordPreviewTitle}, Arguments: []string{argumentText, argumentText}},
	"preview.record_empty":         {Words: langcopy.Phrase{En: "%s in Margince", De: "%s in Margince", Vi: "%s trong Margince"}, Arguments: []string{argumentText}},
	"preview.deals":                {Words: langcopy.Phrase{En: dealCaption, De: dealCaption, Vi: dealCaption}, Arguments: []string{}},
	"preview.companies":            {Words: langcopy.Phrase{En: "Company", De: "Unternehmen", Vi: "Công ty"}, Arguments: []string{}},
	"preview.contacts":             {Words: langcopy.Phrase{En: "Contact", De: "Kontakt", Vi: "Liên hệ"}, Arguments: []string{}},
}
