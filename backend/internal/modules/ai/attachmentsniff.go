// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Do the bytes carry the kind the attachment claims?
//
// An attachment's media type is the UPLOADER's claim, recorded at upload and
// never verified. That is the right answer for storage — the stored content
// type stays the authority for every other reader, and documentextractrun.go
// protects that separation deliberately — and the wrong one for the moment the
// bytes become a wire part addressed to somebody else's decoder.
//
// What this closes: an 8 MB blob labelled image/png, which passed carriage,
// went out with that media_type and came back a vendor 400 with the cost
// already spent; and image/svg+xml, which matches `image/*` by prefix on the
// three operator-pointed wires that still carry a wildcard, and is a TEXT
// format no vision model reads as an image. A carriage check alone admits both:
// it reads the label.
//
// What it deliberately does NOT do: decide anything beyond carriage. It never
// replaces the claim, never rewrites it, and answers "" — no finding — for
// every byte string it cannot speak to.

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// neverCarried are media types no wire in this build carries, whatever its
// declaration says — the narrowing that has to happen by NAME because no reading
// of the bytes can produce it.
//
// One entry, and the argument for it is the format rather than the file:
// image/svg+xml is a vector DOCUMENT with a script surface, it matches `image/*`
// by prefix on the three wires that still declare a wildcard, and no vision
// model decodes it as an image. The sniff below catches an SVG sent as text,
// which is how one usually arrives; it does not catch a gzipped one, and the
// reason to refuse is the same either way.
//
// A set rather than a condition because a second entry is a one-line argument in
// a pull request, which is the right price for widening what a wire refuses.
var neverCarried = []string{"image/svg+xml"}

// mimeBMP is here rather than in carriage.go's block, which is written from the
// adapters' declarations: no adapter names BMP, and none should — no vendor
// decodes it. It is checkable, though, so a BMP claim on one of the three
// wildcard wires gets the same reading as a PNG claim does.
const mimeBMP = "image/bmp"

// sniffableKinds are the media types net/http's sniffer names from magic bytes,
// among the kinds an attachment on these wires can plausibly claim.
//
// The set matters in ONE direction: a claim inside it is checkable, so failing
// to sniff as itself is a finding. A claim outside it (image/tiff, image/avif,
// the HEIF family below) is not checkable this way, and the absence of a
// finding there is honest rather than permissive.
//
// Held by TestEverySniffableKindIsOneTheStdlibActuallyNames, which feeds each
// one its real magic — the set is a claim about a dependency's behaviour, and a
// claim about somebody else's code is exactly the kind that rots quietly.
var sniffableKinds = []string{mimeJPEG, mimePNG, mimeGIF, mimeWebP, mimeBMP, mimePDF}

// heifBrands are the ISO-BMFF brands that make a `ftyp` box HEIC or HEIF — the
// two Gemini documents and the stdlib sniffer does not know.
//
// Without them a genuine HEIC sniffs as application/octet-stream and would pass
// through unchecked, which is the honest outcome; they are here so the one
// vendor that carries the format gets the same check as the others rather than
// a permanent exemption.
var heifBrands = []string{"heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1"}

// ftypOffset is where an ISO-BMFF file's box type sits: a four-byte big-endian
// box length, then the four-character type. The brand follows it.
const (
	ftypOffset  = 4
	brandOffset = 8
	brandEnd    = 12
)

// mislabelledAttachment reports why an attachment's inline bytes are not the
// kind it claims, and "" when nothing here can say they are not.
//
// Three answers and only three, in the order they are asked:
//
//   - A claim the sniffer can check must check out. This is the blob case, and
//     it is the one that needs no judgement: PNG, JPEG, GIF, WebP, BMP and PDF
//     each have an unambiguous signature, so bytes claiming to be one and
//     carrying no signature at all are not that thing.
//   - A HEIC or HEIF claim must carry a HEIF brand, by the same argument
//     through a different reader.
//   - Any other IMAGE claim is refused only when the bytes are TEXT. That is
//     the SVG case, stated as what is actually wrong with it: a decoder
//     expecting an image is being handed prose. It leaves image/tiff and
//     image/avif alone, because an operator-pointed wire may serve a model that
//     reads them and this cannot tell.
func mislabelledAttachment(a model.Attachment) string {
	if len(a.Bytes) == 0 {
		return "" // a URI part carries no bytes here to disagree with
	}
	claimed := mediaKind(a.MIME)
	sniffed := mediaKind(http.DetectContentType(a.Bytes))
	switch {
	case containsFold(sniffableKinds, claimed):
		if sniffed == claimed {
			return ""
		}
		return "the bytes are " + sniffed
	case claimed == mimeHEIC || claimed == mimeHEIF:
		if isHEIF(a.Bytes) {
			return ""
		}
		return "the bytes carry no HEIF brand"
	case isImage(claimed) && strings.HasPrefix(sniffed, "text/"):
		return "the bytes are text (" + sniffed + "), which no vision model decodes as an image"
	default:
		return ""
	}
}

// isHEIF reports whether the bytes open an ISO-BMFF file with a HEIF brand.
func isHEIF(body []byte) bool {
	if len(body) < brandEnd {
		return false
	}
	if !bytes.Equal(body[ftypOffset:brandOffset], []byte("ftyp")) {
		return false
	}
	return containsFold(heifBrands, string(body[brandOffset:brandEnd]))
}

// mediaKind is a media type without its parameters, lowercased — what two of
// them have to agree on. `http.DetectContentType` answers with a charset on
// every text type, and a stored claim may carry one too.
func mediaKind(mime string) string {
	kind, _, _ := strings.Cut(mime, ";")
	return strings.ToLower(strings.TrimSpace(kind))
}

// containsFold is slices.Contains over media types, which are case-insensitive.
func containsFold(set []string, want string) bool {
	for _, have := range set {
		if strings.EqualFold(have, want) {
			return true
		}
	}
	return false
}
