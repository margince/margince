// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The set is a claim about net/http's sniffer, and a claim about somebody
// else's code is the kind that rots without saying so. If the stdlib ever stops
// naming one of these, the check over it silently becomes "refuse everything of
// that kind" — every honest PNG would be read as a mislabelled one.
func TestEverySniffableKindIsOneTheStdlibActuallyNames(t *testing.T) {
	t.Parallel()
	for _, kind := range sniffableKinds {
		sample := sampleBytesFor(kind)
		if sample == nil {
			t.Fatalf("%s is declared sniffable and this package has no sample of it — the set cannot be checked", kind)
		}
		if got := mediaKind(http.DetectContentType(sample)); got != kind {
			t.Errorf("net/http sniffs this package's %s sample as %s — either the sample is wrong or the "+
				"stdlib no longer names the kind, and both make the check over it refuse honest bytes", kind, got)
		}
	}
}

// The HEIF family is the other half of that statement: the stdlib does NOT name
// it, which is why this package reads the brand itself. A test asserting the
// reader works is not enough — it has to assert why the reader exists, because
// the day the stdlib learns HEIC the local reader becomes a second answer.
func TestTheStdlibDoesNotNameHEICSoThisPackageReadsTheBrand(t *testing.T) {
	t.Parallel()
	if got := mediaKind(http.DetectContentType(heicSample)); got == mimeHEIC {
		t.Fatalf("net/http now names HEIC itself — drop isHEIF and add %s to sniffableKinds rather than "+
			"keeping two readers of one question", mimeHEIC)
	}
	if !isHEIF(heicSample) {
		t.Error("the HEIF brand reader does not recognise this package's own HEIC sample")
	}
}

func TestMislabelledAttachment(t *testing.T) {
	t.Parallel()
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	blob := []byte{0x1f, 0x2e, 0x3d, 0x4c, 0x5b, 0x6a, 0x79, 0x88, 0x97, 0xa6, 0xb5, 0xc4}

	for _, c := range []struct {
		name    string
		att     model.Attachment
		refused bool
	}{
		{"a png that is a png", model.Attachment{MIME: mimePNG, Bytes: pngSample}, false},
		{"a pdf that is a pdf", model.Attachment{MIME: mimePDF, Bytes: pdfSample}, false},
		{"a heic that carries the brand", model.Attachment{MIME: mimeHEIC, Bytes: heicSample}, false},

		// THE blob case the ticket names: it passed carriage, went out with
		// that media_type and came back a vendor 400, cost already spent.
		{"a blob labelled png", model.Attachment{MIME: mimePNG, Bytes: blob}, true},
		// THE svg case: `image/*` matches it by prefix on the three wildcard
		// wires, and no vision model decodes text.
		{"an svg labelled as an image", model.Attachment{MIME: "image/svg+xml", Bytes: svg}, true},
		{"a jpeg that is really a png", model.Attachment{MIME: mimeJPEG, Bytes: pngSample}, true},
		{"a pdf that is really an image", model.Attachment{MIME: mimePDF, Bytes: pngSample}, true},
		{"a heic that is not iso-bmff at all", model.Attachment{MIME: mimeHEIC, Bytes: blob}, true},

		// The limits, stated as cases so narrowing them later is a decision
		// somebody makes rather than a side effect.
		{"a uri part has no bytes to disagree with", model.Attachment{MIME: mimePNG, URI: "file-1"}, false},
		{
			"an unrecognised binary claiming a kind the sniffer cannot name",
			model.Attachment{MIME: "image/avif", Bytes: blob},
			false,
		},
		{"a media type nothing here reads", model.Attachment{MIME: "audio/ogg", Bytes: blob}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			finding := mislabelledAttachment(c.att)
			if refused := finding != ""; refused != c.refused {
				t.Fatalf("mislabelledAttachment = %q, want refused: %v", finding, c.refused)
			}
		})
	}
}

// The refusal reaches the caller as its own sentinel, and NOT as the carriage
// one: a caller routing on ErrAttachmentUnsupported falls back to another lane,
// and every lane reads these bytes the same way.
func TestMislabelledBytesRefuseWithTheirOwnSentinel(t *testing.T) {
	t.Parallel()
	err := attachmentUnsupported("fake", []model.Attachment{{MIME: mimePNG, Bytes: []byte("not a png at all")}},
		[]string{mimeAnyImage})
	if err == nil {
		t.Fatal("text labelled image/png must not reach a wire")
	}
	if !errors.Is(err, model.ErrAttachmentMislabelled) {
		t.Errorf("want ErrAttachmentMislabelled, got %v", err)
	}
	if errors.Is(err, model.ErrAttachmentUnsupported) {
		t.Error("a mislabelled attachment must not read as a carriage limit — another lane would be tried " +
			"and would refuse it for the same reason")
	}
}

// The narrowing that cannot be done by reading bytes: an SVG is refused for
// being an SVG, which is what catches the gzipped one the sniff cannot see.
func TestNoWireCarriesAnSVGHoweverItsBytesLook(t *testing.T) {
	t.Parallel()
	gzipped := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
	if finding := mislabelledAttachment(model.Attachment{MIME: "image/svg+xml", Bytes: gzipped}); finding != "" {
		t.Fatalf("the sniff was expected to have nothing to say about a gzipped svg, said %q — if it can "+
			"see this case, refusing by name is a second answer to one question", finding)
	}
	err := attachmentUnsupported("ollama", []model.Attachment{{MIME: "image/svg+xml", Bytes: gzipped}},
		[]string{mimeAnyImage})
	if !errors.Is(err, model.ErrAttachmentUnsupported) {
		t.Errorf("a wildcard wire must still refuse an svg, got %v", err)
	}
}
