// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// MaxImageBytes bounds the image one reading sends. Inline bytes are base64 on
// every carrying adapter, so this is about 10.7 MB of request body. That is an
// order of magnitude under the vendors' ceilings. A larger image is refused
// and never cut, because half a card read confidently is worse than none.
const MaxImageBytes = 8 << 20

// ErrReadRefused marks a reading that cannot succeed with this image. The model
// answered with something this product may not act on, or the bytes could not
// be sent. A retry gets the same answer, so a caller closes the reading.
var ErrReadRefused = errors.New("ai: the reading could not be used")

// ImageNotCarriedError reports that no binding on the lane reads this image
// type. The image is intact and the binding works; they do not meet. Nothing
// was sent, so no failed call sits in the operator's trace for a lane that is
// merely text-only.
type ImageNotCarriedError struct{ MIME string }

func (e *ImageNotCarriedError) Error() string {
	return fmt.Sprintf("ai: the configured model cannot read %s images", e.MIME)
}

// ImageLane is a model lane that says what it can carry before it is asked.
type ImageLane interface {
	Completer
	// AttachmentMIMEs is what a caller may hand this lane, in
	// model.CarriesMIME's spelling. Empty means no image can go to it.
	AttachmentMIMEs() []string
}

// ImageReading is one image in, structured fields out.
type ImageReading struct {
	Image model.Attachment
	// System states the task. The data boundary for the image is appended here.
	System string
	// Instruction is the user turn's text; the image rides beside it.
	Instruction string
	Schema      json.RawMessage
	// Validate is the caller's own read of the reply and runs on every attempt.
	Validate Validator
	// Subject is the record the image belongs to, when it has one. An image
	// that no record holds yet (a photo about to become contacts) leaves it
	// zero, and the call then cites nothing.
	Subject ids.Ref
}

// ReadImage puts one image to the model and returns the validated reply.
//
// Steps: bound the bytes, convert HEIC and HEIF to JPEG, check carriage before
// any call, add the prompt boundary, strip secrets, ask with the validator.
// Three outcomes stay apart. They are a reply, *ImageNotCarriedError and
// ErrReadRefused. Any other error is the lane's.
func ReadImage(ctx context.Context, lane ImageLane, in ImageReading) (json.RawMessage, error) {
	img, err := readableImage(ctx, lane, in.Image)
	if err != nil {
		return nil, err
	}
	req := ImageRequest(in, img)
	if !in.Subject.ID.IsZero() {
		ctx = WithSubject(ctx, in.Subject, "")
	}
	resp, err := Ask(ctx, lane, req, in.Validate)
	if err != nil {
		return nil, RefusedReading(err)
	}
	if err := in.Validate(resp.Text); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadRefused, err)
	}
	return json.RawMessage(resp.Text), nil
}

// ImageRequest is the request one image reading sends. It is a pure function
// of the reading, so a certification case issues the shipping request.
//
//promptlang:exempt the reply is values copied out of an image in the language printed on it, so instructing a language would ask the model to translate what has to match the print.
//promptvoice:exempt the reply is fields copied out of an image, never prose a reader reads as Margince's voice.
func ImageRequest(in ImageReading, img model.Attachment) model.Request {
	fence := promptfence.New()
	return model.Request{
		System: in.System + "\n" + fence.Rule("image"),
		Messages: []model.Message{{
			Role:    "user",
			Content: in.Instruction + "\n" + fence.WrapAttr("image", "media_type", img.MIME),
		}},
		MaxTokens:      ReasoningOutputMaxTokens,
		ResponseSchema: in.Schema,
		SecretStripper: NewSecretStripper(),
		Attachments:    []model.Attachment{img},
	}
}

// readableImage bounds the image, checks that the lane can carry what will be
// sent, and only then turns a HEIC or HEIF photo into JPEG. A lane that reads no
// images never costs a decode.
func readableImage(ctx context.Context, lane ImageLane, img model.Attachment) (model.Attachment, error) {
	if len(img.Bytes) > MaxImageBytes {
		return model.Attachment{}, fmt.Errorf("%w: the image is larger than %d MB", ErrReadRefused, MaxImageBytes>>20)
	}
	carried := lane.AttachmentMIMEs()
	if !isHEIF(img.Bytes) {
		if !model.CarriesMIME(carried, img.MIME) {
			return model.Attachment{}, &ImageNotCarriedError{MIME: img.MIME}
		}
		return img, nil
	}
	if !model.CarriesMIME(carried, mimeJPEG) {
		return model.Attachment{}, &ImageNotCarriedError{MIME: img.MIME}
	}
	jpeg, err := imagenorm.HEICToJPEG(ctx, img.Bytes)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return model.Attachment{}, err
	}
	if err != nil {
		return model.Attachment{}, fmt.Errorf("%w: %w", ErrReadRefused, model.ErrAttachmentMislabelled)
	}
	return model.Attachment{MIME: mimeJPEG, Bytes: jpeg, Name: img.Name}, nil
}

// RefusedReading maps three answers onto ErrReadRefused. One is a binding
// refusing a type it declared. One is bytes that are not the type they claim.
// One is a reply the model withheld or the validator rejected. Any other error
// is returned as it is. That means the lane is down, so retry.
func RefusedReading(err error) error {
	switch {
	case errors.Is(err, model.ErrAttachmentUnsupported):
		return fmt.Errorf("%w: the model refused a type its binding declares it carries: %w", ErrReadRefused, err)
	case errors.Is(err, model.ErrAttachmentMislabelled):
		return fmt.Errorf("%w: the bytes are not the type they are stored as: %w", ErrReadRefused, err)
	case ModelDeclined(err):
		return fmt.Errorf("%w: %w", ErrReadRefused, err)
	default:
		return err
	}
}

// SniffImage names the image type of the bytes from their magic. It reports
// false for anything but JPEG, PNG, GIF, WebP, HEIC and HEIF. The caller's
// claim about the type is not an input. The bytes decide.
func SniffImage(body []byte) (mime string, ok bool) {
	if isHEIF(body) {
		return mimeHEIC, true
	}
	switch kind := mediaKind(http.DetectContentType(body)); kind {
	case mimeJPEG, mimePNG, mimeGIF, mimeWebP:
		return kind, true
	default:
		return "", false
	}
}

// CarriesImage reports whether mime is a photo type a reading accepts, which is
// the set SniffImage names.
func CarriesImage(mime string) bool {
	switch mediaKind(mime) {
	case mimeJPEG, mimePNG, mimeGIF, mimeWebP, mimeHEIC, mimeHEIF:
		return true
	default:
		return false
	}
}
