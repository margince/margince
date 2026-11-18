// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/htmltext"
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
		{"a price with the currency code first", "Re: call", "We can offer annual support for EUR 900.", nil, true},
		{"a price in dollars, code first", "Re: call", "Support is USD 1,200 a year.", nil, true},
		{"a price in dong, code first", "Re: lịch hẹn", "Phí là VND 5.000.000.", nil, true},
		{"a reply that offers a service", "Re: call", "We offer onsite training as well.", nil, true},
		{"a German reply that offers a service", "Re: Termin", "Wir bieten Ihnen eine Schulung vor Ort an.", nil, true},
		{"a Vietnamese reply that offers a service", "Re: lịch hẹn", "Chúng tôi cung cấp đào tạo tại chỗ.", nil, true},
		{"a request to quote a section", "Re: contract", "Please quote section 4 in your answer.", nil, false},
		{"a price quote", "Re: call", "Attached is the price quote you asked for.", nil, true},
		{"a fee named in passing", "Re: filing", "The filing fee was €20.", nil, true},
		{"a currency code without an amount", "Re: call", "We invoice in EUR as agreed.", nil, false},
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
	if withAuthoredOffer(neutral, "").Evidence.AuthoredOffer {
		t.Fatal("a neutral message read as an offer: the fixture proves nothing")
	}
	pitched := neutral
	pitched.AuthoredHTML = `<p>Siehe unten.</p><style>p{color:red}</style><p>Ihr <b>Sonder&shy;ange&shy;bot</b> f&uuml;r 2027</p>`
	if !withAuthoredOffer(pitched, "").Evidence.AuthoredOffer {
		t.Error("an offer in the markup alone did not read as an offer")
	}
}

// Markup the renderer stopped reading is judged as an offer, so a pitch past
// the cap cannot hide there.
func TestMarkupPastTheRenderCapIsJudgedAsAnOffer(t *testing.T) {
	long := strings.Repeat("<p>Können wir den Termin verschieben?</p>", htmltext.MaxRenderedLen/30)
	req := commsauthz.Request{AuthoredBody: "Siehe unten.", AuthoredHTML: long + "<p>Unser Angebot</p>"}
	if !withAuthoredOffer(req, "").Evidence.AuthoredOffer {
		t.Error("a pitch past the render cap did not read as an offer")
	}
}

// A subject carried over from the message answered is left for the
// recipient to settle. A subject the rep changed is read as written.
func TestACarriedOverSubjectIsLeftForTheRecipientToSettle(t *testing.T) {
	const anchor = "Termin und Angebot"
	for _, c := range []struct {
		subject       string
		offer, quoted bool
	}{
		{"Re: Termin und Angebot", false, true},
		{"AW: re:  termin und angebot", false, true},
		{"WG: Termin und Angebot", false, true},
		{"Re: Termin und Angebot mit 20 % Rabatt", true, false},
		{"Unser Angebot", true, false},
	} {
		req := commsauthz.Request{AuthoredSubject: c.subject, AuthoredBody: "Können wir den Termin verschieben?"}
		got := withAuthoredOffer(req, anchor).Evidence
		if got.AuthoredOffer != c.offer || got.QuotedSubjectOffer != c.quoted {
			t.Errorf("%q answering %q: offer %v, quoted %v; want %v, %v", c.subject, anchor, got.AuthoredOffer, got.QuotedSubjectOffer, c.offer, c.quoted)
		}
	}
	if got := withAuthoredOffer(commsauthz.Request{AuthoredSubject: "Re: Termin und Angebot"}, "").Evidence; !got.AuthoredOffer || got.QuotedSubjectOffer {
		t.Errorf("a subject naming an offer with no message answered reads %+v, want an authored offer", got)
	}
}
