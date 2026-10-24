// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for document_extract/cards.
//
// It certifies the shipped path. The request is ai.ImageRequest over
// cardPhotoReading. The reply goes through the import's own validator and
// confidence floor. A photo is right or wrong about who is on the cards, so the
// expectation is the list of full names printed on them. An empty list is the
// abstention that matters. A photo with no card must not yield a contact.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const laneCards = "cards"

// cardPhotoFixture is one photo in the form the reading hands the model.
type cardPhotoFixture struct {
	MIME     string `json:"mime"`
	Bytes    []byte `json:"bytes"`
	Filename string `json:"filename"`
}

type cardPhotoCases struct{}

func (cardPhotoCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskDocumentExtract, Variant: laneCards, Kind: ai.SiteKindOneShot}
}

// CertifiedScope is the site's own kind: a photo is read in one call.
func (cardPhotoCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (cardPhotoCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f cardPhotoFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("document_extract/cards: the fixture is not the shape this site takes: %w", err)
	}
	if len(f.Bytes) == 0 || len(f.Bytes) > imagenorm.MaxReadImageBytes {
		return nil, fmt.Errorf("document_extract/cards: the fixture must be a photo of 1 to %d bytes", imagenorm.MaxReadImageBytes)
	}
	if !ai.CarriesImage(f.MIME) {
		return nil, fmt.Errorf("document_extract/cards: %q is not a photo type a reading accepts", f.MIME)
	}
	var names []string
	if err := json.Unmarshal(expected, &names); err != nil {
		return nil, fmt.Errorf("document_extract/cards: the expected answer is the list of full names printed on the cards: %w", err)
	}
	if len(names) > contacts.CardPhotoMaxCards {
		return nil, fmt.Errorf("document_extract/cards: the scenario expects %d cards, and one photo yields at most %d", len(names), contacts.CardPhotoMaxCards)
	}
	return &cardPhotoCase{photo: model.Attachment{MIME: f.MIME, Bytes: f.Bytes, Name: f.Filename}, names: names}, nil
}

type cardPhotoCase struct {
	photo model.Attachment
	names []string
}

// Run issues the one request this site sends, bare, as the other one-call
// sites do.
func (c *cardPhotoCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := ai.ImageRequest(cardPhotoReading(c.photo), c.photo)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("document_extract/cards: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the import's own validator, then compares the names of the
// cards the floor lets through with the names the scenario expects.
func (c *cardPhotoCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	if err := cardPhotoShapeValid(trace.Output); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	var payload cardPhotoPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil || payload.Cards == nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: "the reply does not parse"}
	}
	var got []string
	for _, card := range *payload.Cards {
		if float64(card.Confidence) >= cardConfidenceFloor {
			got = append(got, strings.TrimSpace(card.FullName))
		}
	}
	want := slices.Clone(c.names)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("the reading offers %q where the photo shows %q", got, want),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
