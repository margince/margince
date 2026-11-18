// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "testing"

// Each pair shares its routine text, so the one difference is whether the
// sender wrote an offer.
func TestAnOfferCountsOnlyWhenTheSenderWroteIt(t *testing.T) {
	const reschedule = "Hallo Frau Buyer,\n\nKönnen wir den Termin auf Donnerstag, 14 Uhr verschieben?\n\nViele Grüße\nAn Tran"
	for _, c := range []struct {
		name, subject, body string
		want                bool
	}{
		{"a German reschedule", "Re: Termin", reschedule, false},
		{"the reschedule above a quoted offer", "Re: Termin", reschedule + "\n\n> Unser Angebot: 20 % Rabatt", false},
		{"the reschedule with an authored offer", "Re: Termin", reschedule + "\n\nAnbei unser Angebot mit 20 % Rabatt.", true},
		{"a sign-off alone", "Re: Termin", "Best regards,\nAn Tran", false},
		{"an English reply", "Re: call", "Could we move our call to Friday at 3pm?", false},
		{"the English reply with a discount", "Re: call", "Could we move our call to Friday? We can also offer a 15% off renewal.", true},
		{"the English reply with an offer in the subject", "Our offer for next year", "Could we move our call to Friday?", true},
		{"a Vietnamese reply", "Re: lịch hẹn", "Chúng ta có thể dời cuộc họp sang thứ Sáu không?", false},
		{"the Vietnamese reply with a price quote", "Re: lịch hẹn", "Chúng ta có thể dời cuộc họp sang thứ Sáu không? Anh xem báo giá đính kèm.", true},
		{"a price in euro", "Re: Termin", "Der Preis liegt bei 1.200 €.", true},
		{"a price in dong", "Re: lịch hẹn", "Giá chỉ 500.000đ.", true},
		{"a count of orders in Vietnamese", "Re: lịch hẹn", "Chúng tôi đã nhận 2 đơn hàng.", false},
		{"a price in dollars", "Re: call", "It comes to $ 900 per seat.", true},
		{"a time and a date", "Re: call", "See you on 14.11.2026 at 10:30.", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := authoredOffer(c.subject, c.body); got != c.want {
				t.Errorf("authoredOffer = %v, want %v", got, c.want)
			}
		})
	}
}
