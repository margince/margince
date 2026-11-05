// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type imageLane struct {
	carries []string
	reply   string
	err     error
	calls   int
	got     model.Request
}

func (l *imageLane) AttachmentMIMEs() []string { return l.carries }

func (l *imageLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.calls++
	l.got = req
	return model.Response{Text: l.reply}, l.err
}

func okReply(string) error { return nil }

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reading(img model.Attachment) ImageReading {
	return ImageReading{Image: img, System: "read", Instruction: "go", Schema: []byte(`{}`), Validate: okReply}
}

func TestReadImageNamesATextOnlyBindingWithoutCallingTheModel(t *testing.T) {
	lane := &imageLane{}
	_, err := ReadImage(context.Background(), lane, reading(model.Attachment{MIME: mimePNG, Bytes: pngBytes(t)}))
	var notCarried *ImageNotCarriedError
	if !errors.As(err, &notCarried) || notCarried.MIME != mimePNG {
		t.Fatalf("err = %v, want ImageNotCarriedError naming image/png", err)
	}
	if lane.calls != 0 {
		t.Fatalf("the model was called %d times for a type its binding cannot carry", lane.calls)
	}
}

func TestReadImageSeparatesARefusedReplyFromALaneFault(t *testing.T) {
	img := model.Attachment{MIME: mimePNG, Bytes: pngBytes(t)}

	invalid := reading(img)
	invalid.Validate = func(string) error { return errors.New("no") }
	_, err := ReadImage(context.Background(), &imageLane{carries: []string{mimePNG}, reply: "x"}, invalid)
	if !errors.Is(err, ErrReadRefused) {
		t.Errorf("an invalid reply: err = %v, want ErrReadRefused", err)
	}

	down := errors.New("connection reset")
	_, err = ReadImage(context.Background(), &imageLane{carries: []string{mimePNG}, err: down}, reading(img))
	if !errors.Is(err, down) || errors.Is(err, ErrReadRefused) {
		t.Errorf("a lane fault: err = %v, want it returned unchanged for a retry", err)
	}

	_, err = ReadImage(context.Background(), &imageLane{carries: []string{mimePNG}, err: model.ErrAttachmentMislabelled}, reading(img))
	if !errors.Is(err, ErrReadRefused) || !errors.Is(err, model.ErrAttachmentMislabelled) {
		t.Errorf("mislabelled bytes: err = %v, want a refusal that still names the cause", err)
	}
}

func TestReadImageRefusesAnImageOverTheBound(t *testing.T) {
	lane := &imageLane{carries: []string{mimePNG}}
	_, err := ReadImage(context.Background(), lane, reading(model.Attachment{MIME: mimePNG, Bytes: make([]byte, imagenorm.MaxReadImageBytes+1)}))
	if !errors.Is(err, ErrImageTooLarge) || !errors.Is(err, ErrReadRefused) || lane.calls != 0 {
		t.Fatalf("err = %v, calls = %d", err, lane.calls)
	}
}

func TestImageRequestTellsTheModelThePrintHasNoAuthority(t *testing.T) {
	img := model.Attachment{MIME: mimePNG, Bytes: pngBytes(t)}
	req := ImageRequest(reading(img), img)

	if !strings.Contains(req.System, imageTextRule) {
		t.Errorf("system prompt %q lacks the rule for text printed in the image", req.System)
	}
	if strings.Contains(req.System+req.Messages[0].Content, "untrusted-") {
		t.Errorf("the request declares a fence, and no fence can bound an attached image")
	}
}

func TestSniffImageDecidesFromTheBytes(t *testing.T) {
	cases := map[string]struct {
		body []byte
		want string
	}{
		"png":  {pngBytes(t), mimePNG},
		"heic": {[]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), mimeHEIC},
		"text": {[]byte("BEGIN:VCARD"), ""},
	}
	for name, c := range cases {
		got, ok := SniffImage(c.body)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s: got %q ok=%v, want %q", name, got, ok, c.want)
		}
	}
}
