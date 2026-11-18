// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Each pair shares its routine text, so the one difference is whether the
// sender wrote an offer.
func TestAnOfferCountsWhereverTheSenderWroteIt(t *testing.T) {
	const reschedule = "Hallo Frau Buyer,\n\nKönnen wir den Termin auf Donnerstag, 14 Uhr verschieben?\n\nViele Grüße\nAn Tran"
	for _, c := range []struct {
		name, subject, body string
		files               []string
		want                bool
	}{
		{"a German reschedule", "Re: Termin", reschedule, nil, false},
		{"the reschedule with an authored offer", "Re: Termin", reschedule + "\n\nAnbei unser Angebot mit 20 % Rabatt.", nil, true},
		{"the reschedule with an offer behind a quote marker", "Re: Termin", reschedule + "\n\n> Unser Angebot: 20 % Rabatt", nil, true},
		{"the reschedule with a compound discount", "Re: Termin", reschedule + "\n\nIhr Sonderrabatt gilt bis Freitag.", nil, true},
		{"the reschedule with a compound offer price", "Re: Termin", reschedule + "\n\nDer Angebotspreis steht fest.", nil, true},
		{"the reschedule with a discount campaign", "Re: Termin", reschedule + "\n\nUnsere Rabattaktion läuft noch.", nil, true},
		{"the reschedule about a transaction", "Re: Termin", reschedule + "\n\nDie Transaktion ist gebucht, die Redaktion informiert.", nil, false},
		{"the reschedule with an offer attached", "Re: Termin", reschedule + "\n\nSiehe Anhang.", []string{"Angebot_2026.pdf"}, true},
		{"the reschedule with an agenda attached", "Re: Termin", reschedule + "\n\nSiehe Anhang.", []string{"Agenda_2026.pdf"}, false},
		{"a sign-off alone", "Re: Termin", "Best regards,\nAn Tran", nil, false},
		{"an English reply", "Re: call", "Could we move our call to Friday at 3pm?", nil, false},
		{"the English reply with a discount", "Re: call", "Could we move our call to Friday? We can also offer a 15% off renewal.", nil, true},
		{"the English reply with an offer in the subject", "Our offer for next year", "Could we move our call to Friday?", nil, true},
		{"a Vietnamese reply", "Re: lịch hẹn", "Chúng ta có thể dời cuộc họp sang thứ Sáu không?", nil, false},
		{"the Vietnamese reply with a price quote", "Re: lịch hẹn", "Chúng ta có thể dời cuộc họp sang thứ Sáu không? Anh xem báo giá đính kèm.", nil, true},
		{"a price in euro", "Re: Termin", "Der Preis liegt bei 1.200 €.", nil, true},
		{"a price in dong", "Re: lịch hẹn", "Giá chỉ 500.000đ.", nil, true},
		{"a count of orders in Vietnamese", "Re: lịch hẹn", "Chúng tôi đã nhận 2 đơn hàng.", nil, false},
		{"a price in dollars", "Re: call", "It comes to $ 900 per seat.", nil, true},
		{"a time and a date", "Re: call", "See you on 14.11.2026 at 10:30.", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := authoredOffer(c.subject, c.body, c.files); got != c.want {
				t.Errorf("authoredOffer = %v, want %v", got, c.want)
			}
		})
	}
}

// A pitch that sits only in the markup is an offer: a client shows the markup,
// so its visible text is what the recipient reads.
func TestAnOfferInTheMarkupAloneIsAnOffer(t *testing.T) {
	neutral := commsauthz.Request{AuthoredSubject: "Re: Termin", AuthoredBody: "Siehe unten."}
	if withAuthoredOffer(neutral).Evidence.AuthoredOffer {
		t.Fatal("a neutral message read as an offer: the fixture proves nothing")
	}
	pitched := neutral
	pitched.AuthoredHTML = `<p>Siehe unten.</p><style>p{color:red}</style><p>Ihr <b>Sonder&shy;ange&shy;bot</b> f&uuml;r 2027</p>`
	if !withAuthoredOffer(pitched).Evidence.AuthoredOffer {
		t.Error("an offer in the markup alone did not read as an offer")
	}
}
