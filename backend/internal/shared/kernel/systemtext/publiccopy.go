// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var publicDefinitions = map[string]Definition{
	"booking.lets_meet": {Words: langcopy.Phrase{En: "Let's meet", De: "Treffen wir uns", Vi: "Hãy gặp nhau"}},
	"booking.meeting":   {Words: langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"}},
	"booking.proposed":  {Words: langcopy.Phrase{En: "Meeting proposed: %s", De: "Vorgeschlagener Termin: %s", Vi: "Đề xuất cuộc họp: %s"}, Arguments: []string{argumentText}},

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
