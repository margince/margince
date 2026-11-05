// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package receiptcopy names the source of a cited exchange.
package receiptcopy

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type channelWords struct{ Neutral, Outbound, Inbound langcopy.Phrase }

var channels = map[string]channelWords{
	"email": {
		langcopy.Phrase{En: "Email", De: "E-Mail", Vi: "Email"},
		langcopy.Phrase{En: "Email you sent", De: "Von dir gesendete E-Mail", Vi: "Email bạn đã gửi"},
		langcopy.Phrase{En: "Email they sent", De: "Empfangene E-Mail", Vi: "Email họ đã gửi"},
	},
	"message": {
		langcopy.Phrase{En: "Message", De: "Nachricht", Vi: "Tin nhắn"},
		langcopy.Phrase{En: "Message you sent", De: "Von dir gesendete Nachricht", Vi: "Tin nhắn bạn đã gửi"},
		langcopy.Phrase{En: "Message they sent", De: "Empfangene Nachricht", Vi: "Tin nhắn họ đã gửi"},
	},
	"call": {
		langcopy.Phrase{En: "Call", De: "Anruf", Vi: "Cuộc gọi"},
		langcopy.Phrase{En: "Call you made", De: "Von dir getätigter Anruf", Vi: "Cuộc gọi bạn đã thực hiện"},
		langcopy.Phrase{En: "Call they made", De: "Eingegangener Anruf", Vi: "Cuộc gọi họ đã thực hiện"},
	},
	"meeting": {
		langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"},
		langcopy.Phrase{En: "Meeting you held", De: "Von dir veranstaltete Besprechung", Vi: "Cuộc họp bạn đã tổ chức"},
		langcopy.Phrase{En: "Meeting they held", De: "Von ihnen veranstaltete Besprechung", Vi: "Cuộc họp họ đã tổ chức"},
	},
	"": {
		langcopy.Phrase{En: "Exchange", De: "Austausch", Vi: "Trao đổi"},
		langcopy.Phrase{En: "Sent by you", De: "Von dir gesendet", Vi: "Do bạn gửi"},
		langcopy.Phrase{En: "Sent by them", De: "Von ihnen gesendet", Vi: "Do họ gửi"},
	},
}

// Origin describes who supplied an interaction without changing its source facts.
func Origin(said langcopy.Spoken, kind, direction string) string {
	words, ok := channels[kind]
	if !ok {
		words = channels[""]
	}
	switch direction {
	case "outbound":
		return said.Say(words.Outbound)
	case "inbound":
		return said.Say(words.Inbound)
	default:
		return said.Say(words.Neutral)
	}
}
