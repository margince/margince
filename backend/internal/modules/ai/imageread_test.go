// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

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

func heicFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../platform/imagenorm/testdata/card.heic")
	if err != nil {
		t.Fatal(err)
	}
	return b
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

func TestReadImageConvertsHEICToJPEGSoABindingWithoutHEICCanRead(t *testing.T) {
	lane := &imageLane{carries: []string{mimeJPEG}, reply: `{"ok":true}`}
	raw, err := ReadImage(context.Background(), lane, reading(model.Attachment{MIME: mimeHEIC, Bytes: heicFixture(t)}))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ok":true}` {
		t.Errorf("reply = %s", raw)
	}
	sent := lane.got.Attachments
	if len(sent) != 1 || sent[0].MIME != mimeJPEG {
		t.Fatalf("sent attachments = %+v, want one image/jpeg", sent)
	}
	if _, err := jpeg.Decode(bytes.NewReader(sent[0].Bytes)); err != nil {
		t.Fatalf("the part sent to the model is not a JPEG: %v", err)
	}
}

func TestReadImageRefusesAHEICThatDoesNotDecode(t *testing.T) {
	lane := &imageLane{carries: []string{mimeJPEG}, reply: `{}`}
	_, err := ReadImage(context.Background(), lane, reading(model.Attachment{MIME: mimeHEIC, Bytes: heicFixture(t)[:200]}))
	if !errors.Is(err, ErrReadRefused) || lane.calls != 0 {
		t.Fatalf("err = %v, calls = %d, want a refusal before any call", err, lane.calls)
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
	_, err := ReadImage(context.Background(), lane, reading(model.Attachment{MIME: mimePNG, Bytes: make([]byte, MaxImageBytes+1)}))
	if !errors.Is(err, ErrReadRefused) || lane.calls != 0 {
		t.Fatalf("err = %v, calls = %d", err, lane.calls)
	}
}

func TestSniffImageDecidesFromTheBytes(t *testing.T) {
	cases := map[string]struct {
		body []byte
		want string
	}{
		"png":  {pngBytes(t), mimePNG},
		"heic": {heicFixture(t), mimeHEIC},
		"text": {[]byte("BEGIN:VCARD"), ""},
	}
	for name, c := range cases {
		got, ok := SniffImage(c.body)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s: got %q ok=%v, want %q", name, got, ok, c.want)
		}
	}
}
