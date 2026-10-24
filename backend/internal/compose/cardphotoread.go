// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a photo of business cards into contact cards.
//
// This file owns the question put to the model and what may come back.
// ai.ReadImage owns the shared image steps: HEIC conversion, the carriage
// check, the prompt boundary and the validated ask. What leaves here are cards
// marked ModelRead. The contacts module never writes such a card; it reports
// it for approval, so a misread name needs a human before it is a record.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// cardConfidenceFloor drops a card the model is unsure it read at all. A card
	// that is wrong costs the importer an approval to reject, so unsure ones are
	// left out and counted rather than offered.
	cardConfidenceFloor = 0.5
	// maxCardValue bounds one field. A card prints short lines, and a long value is
	// the model running text together.
	maxCardValue = 300
	// maxCardChannels bounds the emails or phones of one card.
	maxCardChannels = 5
)

const cardPhotoSystem = `You read ONE photograph of business cards and report only what is PRINTED on them.
Return one entry per card you can see, in reading order, with the name, title, company, email
addresses, phone numbers, website and postal address exactly as printed, in the card's own
language and script. Leave a field empty when the card does not print it. Never complete,
translate, correct or infer a value, and never invent an email address from a name and a company.
A photograph with no business card in it is a normal answer: return an empty list.`

const cardPhotoInstruction = `One photograph (untrusted). Return JSON: { "cards": [ { "full_name", "title", "company", "emails", "phones", "url", "address", "confidence" } ] }.
"confidence" is your own certainty, from 0 to 1, that this entry is one card read correctly.`

type cardPhotoCard struct {
	FullName   string            `json:"full_name"`
	Title      string            `json:"title"`
	Company    string            `json:"company"`
	Emails     []string          `json:"emails"`
	Phones     []string          `json:"phones"`
	URL        string            `json:"url"`
	Address    string            `json:"address"`
	Confidence schema.Confidence `json:"confidence"`
}

type cardPhotoPayload struct {
	Cards *[]cardPhotoCard `json:"cards"`
}

func cardPhotoSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"cards": schema.Array(schema.Object(
				map[string]schema.Node{
					"full_name":  schema.String(),
					"title":      schema.String(),
					"company":    schema.String(),
					"emails":     schema.Array(schema.String()),
					"phones":     schema.Array(schema.String()),
					"url":        schema.String(),
					"address":    schema.String(),
					"confidence": schema.Number(),
				},
				"full_name", "title", "company", "emails", "phones", "url", "address", "confidence",
			)),
		},
		"cards",
	))
}

// cardPhotoShapeValid refuses a reply the import may not act on, with a message
// that names the fault so the retry can fix it.
func cardPhotoShapeValid(text string) error {
	var payload cardPhotoPayload
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
		return fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	if payload.Cards == nil {
		return errors.New("the reply carries no cards key; return an empty list when no card is visible")
	}
	for i, card := range *payload.Cards {
		if strings.TrimSpace(card.FullName) == "" {
			return fmt.Errorf("card %d states no name; leave a card out when it prints none", i+1)
		}
		for _, value := range cardValues(card) {
			if utf8.RuneCountInString(value) > maxCardValue {
				return fmt.Errorf("card %d has a value longer than %d characters; report one printed line, not running text", i+1, maxCardValue)
			}
		}
		if len(card.Emails) > maxCardChannels || len(card.Phones) > maxCardChannels {
			return fmt.Errorf("card %d lists more than %d addresses or numbers", i+1, maxCardChannels)
		}
	}
	return nil
}

func cardValues(card cardPhotoCard) []string {
	values := append([]string{card.FullName, card.Title, card.Company, card.URL, card.Address}, card.Emails...)
	return append(values, card.Phones...)
}

// errTooManyCards closes a reading whose photo shows more cards than one photo
// may yield. The list is refused whole: a cut list reads as the full photo.
var errTooManyCards = errors.New("compose: the photo shows more cards than one photo may yield")

// cardPhotoReading is the question put to the model for one photo, a pure
// function so the certification case issues the shipping request.
//
//promptlang:exempt every value returned is copied from the card verbatim, in the card's own language, so instructing a language would ask the model to translate what has to match the print.
//promptvoice:exempt every value returned is copied from the card verbatim.
func cardPhotoReading(img model.Attachment) ai.ImageReading {
	return ai.ImageReading{
		Image:       img,
		System:      cardPhotoSystem,
		Instruction: cardPhotoInstruction,
		Schema:      cardPhotoSchema(),
		Validate:    cardPhotoShapeValid,
	}
}

// readCardPhoto sends the photo to the model and returns the cards, marked
// ModelRead. Cards below the confidence floor are dropped and counted.
func readCardPhoto(ctx context.Context, lane ai.ImageLane, photo []byte, mime string) (cards []contacts.VCardEntry, dropped int, err error) {
	raw, err := ai.ReadImage(ctx, lane, cardPhotoReading(model.Attachment{MIME: mime, Bytes: photo, Name: "card-photo"}))
	if err != nil {
		return nil, 0, err
	}
	var payload cardPhotoPayload
	if err := json.Unmarshal([]byte(ai.Unfence(string(raw))), &payload); err != nil || payload.Cards == nil {
		return nil, 0, fmt.Errorf("%w: the validated reply no longer parses", ai.ErrReadRefused)
	}
	if len(*payload.Cards) > contacts.CardPhotoMaxCards {
		return nil, 0, errTooManyCards
	}
	for _, card := range *payload.Cards {
		if float64(card.Confidence) < cardConfidenceFloor {
			dropped++
			continue
		}
		cards = append(cards, cardEntry(card))
	}
	return cards, dropped, nil
}

// cardEntry is the only place a card becomes a VCardEntry with ModelRead set.
func cardEntry(card cardPhotoCard) contacts.VCardEntry {
	return contacts.VCardEntry{
		FullName:  strings.TrimSpace(card.FullName),
		Company:   strings.TrimSpace(card.Company),
		Title:     strings.TrimSpace(card.Title),
		Emails:    cardChannels(card.Emails, func(v string) bool { return strings.Contains(v, "@") }),
		Phones:    cardChannels(card.Phones, func(string) bool { return true }),
		URL:       strings.TrimSpace(card.URL),
		Address:   strings.TrimSpace(card.Address),
		ModelRead: true,
	}
}

// cardChannels keeps the values worth storing. A card states a working address
// unless it says otherwise, which is the vCard reader's default as well.
func cardChannels(values []string, keep func(string) bool) []contacts.VCardChannel {
	var out []contacts.VCardChannel
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && keep(v) {
			out = append(out, contacts.VCardChannel{Value: v, Kind: "work"})
		}
	}
	return out
}

// cardPhotoReader adapts readCardPhoto to the contacts module's injected port,
// turning each outcome into the refusal a client can act on.
func cardPhotoReader(lane ai.ImageLane) contacts.CardPhotoReader {
	return func(ctx context.Context, photo []byte) ([]contacts.VCardEntry, error) {
		mime, ok := ai.SniffImage(photo)
		if !ok {
			return nil, &contacts.CardPhotoRefusal{Code: "unsupported_type",
				Message: "Send a JPEG, PNG, GIF, WebP, HEIC or HEIF photo."}
		}
		cards, dropped, err := readCardPhoto(ctx, lane, photo, mime)
		var notCarried *ai.ImageNotCarriedError
		switch {
		case errors.As(err, &notCarried):
			return nil, &contacts.CardPhotoRefusal{Code: "not_readable",
				Message: "The configured model cannot read " + notCarried.MIME + " photos. Ask an administrator to configure a model that reads images."}
		case errors.Is(err, ai.ErrReadRefused):
			return nil, &contacts.CardPhotoRefusal{Code: "not_readable",
				Message: "This photo could not be read. Try a sharper photo of the cards."}
		case errors.Is(err, errTooManyCards):
			return nil, &contacts.CardPhotoRefusal{Code: "too_many_cards",
				Message: fmt.Sprintf("The photo shows more than %d cards. Photograph fewer cards at a time.", contacts.CardPhotoMaxCards)}
		case err != nil:
			return nil, err
		case len(cards) == 0:
			return nil, &contacts.CardPhotoRefusal{Code: "no_cards", Message: noCardsMessage(dropped)}
		}
		return cards, nil
	}
}

func noCardsMessage(dropped int) string {
	if dropped > 0 {
		return "No card could be read with confidence. Try a sharper, better lit photo."
	}
	return "The photo shows no business card."
}
