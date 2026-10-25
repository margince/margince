// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type cardLane struct {
	carries []string
	reply   string
	calls   int
}

func (l *cardLane) AttachmentMIMEs() []string { return l.carries }

func (l *cardLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: l.reply}, nil
}

func cardPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const twoCards = `{"cards":[
 {"full_name":" 田中 太郎 ","title":"部長","company":"Müller & Söhne GmbH","emails":["taro@mueller.example","not-an-address"],"phones":["+81 3 1234 5678"],"url":"","address":"","confidence":0.9},
 {"full_name":"Blurry Name","title":"","company":"","emails":[],"phones":[],"url":"","address":"","confidence":0.2}]}`

func TestReadCardPhotoMarksEveryCardModelReadAndDropsTheUnsure(t *testing.T) {
	lane := &cardLane{carries: []string{"image/png"}, reply: twoCards}

	cards, dropped, err := readCardPhoto(context.Background(), lane, cardPNG(t), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 || len(cards) != 1 {
		t.Fatalf("cards = %d, dropped = %d, want 1 and 1", len(cards), dropped)
	}
	card := cards[0]
	if !card.ModelRead {
		t.Error("a card from a photo must be marked ModelRead")
	}
	if card.FullName != "田中 太郎" || card.Company != "Müller & Söhne GmbH" {
		t.Errorf("name/company = %q / %q, want the printed text unchanged", card.FullName, card.Company)
	}
	if len(card.Emails) != 1 || card.Emails[0].Value != "taro@mueller.example" || card.Emails[0].Kind != "work" {
		t.Errorf("emails = %+v, want only the real address", card.Emails)
	}
}

func TestReadCardPhotoRefusesAWholeListAboveTheCap(t *testing.T) {
	var cards []string
	for i := 0; i <= contacts.CardPhotoMaxCards; i++ {
		cards = append(cards, fmt.Sprintf(`{"full_name":"P%d","title":"","company":"","emails":[],"phones":[],"url":"","address":"","confidence":0.9}`, i))
	}
	lane := &cardLane{carries: []string{"image/png"}, reply: `{"cards":[` + strings.Join(cards, ",") + `]}`}

	_, _, err := readCardPhoto(context.Background(), lane, cardPNG(t), "image/png")
	if !errors.Is(err, errTooManyCards) {
		t.Fatalf("err = %v, want errTooManyCards: a cut list reads as the whole photo", err)
	}
}

func TestCardPhotoShapeValidNamesTheFault(t *testing.T) {
	cases := map[string]string{
		"no cards key": `{}`,
		"no name":      `{"cards":[{"full_name":"  ","title":"","company":"","emails":[],"phones":[],"url":"","address":"","confidence":1}]}`,
		"long value":   `{"cards":[{"full_name":"` + strings.Repeat("x", maxCardValue+1) + `","title":"","company":"","emails":[],"phones":[],"url":"","address":"","confidence":1}]}`,
	}
	for name, reply := range cases {
		if err := cardPhotoShapeValid(reply); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := cardPhotoShapeValid(`{"cards":[]}`); err != nil {
		t.Errorf("an empty list is the correct answer for a photo with no card: %v", err)
	}
}

func TestCardPhotoReaderTurnsEachOutcomeIntoARefusalAClientCanAct(t *testing.T) {
	code := func(err error) string {
		var refusal *contacts.CardPhotoRefusal
		if !errors.As(err, &refusal) {
			t.Fatalf("err = %v, want a CardPhotoRefusal", err)
		}
		return refusal.Code
	}
	photo := cardPNG(t)

	_, err := cardPhotoReader(&cardLane{carries: []string{"image/png"}})(context.Background(), []byte("BEGIN:VCARD"))
	if got := code(err); got != "unsupported_type" {
		t.Errorf("a vCard sent as a photo: code = %s", got)
	}

	heic := &cardLane{carries: []string{"image/heic", "image/png"}}
	_, err = cardPhotoReader(heic)(context.Background(), []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"))
	var refusal *contacts.CardPhotoRefusal
	if got := code(err); got != "unsupported_type" || heic.calls != 0 {
		t.Errorf("a HEIC photo: code = %s, calls = %d, want unsupported_type and no model call", got, heic.calls)
	}
	if errors.As(err, &refusal) && !strings.Contains(refusal.Message, "JPEG or PNG") {
		t.Errorf("message = %q, want it to say to save the photo as JPEG or PNG", refusal.Message)
	}

	textOnly := &cardLane{}
	_, err = cardPhotoReader(textOnly)(context.Background(), photo)
	if got := code(err); got != "not_readable" || textOnly.calls != 0 {
		t.Errorf("a text-only binding: code = %s, calls = %d, want not_readable and no call", got, textOnly.calls)
	}

	_, err = cardPhotoReader(&cardLane{carries: []string{"image/png"}, reply: `{"cards":[]}`})(context.Background(), photo)
	if got := code(err); got != "no_cards" {
		t.Errorf("a photo with no card: code = %s", got)
	}

	entries, err := cardPhotoReader(&cardLane{carries: []string{"image/png"}, reply: twoCards})(context.Background(), photo)
	if err != nil || len(entries) != 1 || !entries[0].ModelRead {
		t.Errorf("entries = %+v, err = %v, want one model-read card", entries, err)
	}
}
