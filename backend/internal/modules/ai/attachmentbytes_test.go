// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Fixture bytes that ARE the kind their attachment claims.
//
// Every attachment fixture in this package used to carry a word — `[]byte("PNG")`
// — which reads fine and is text. That was harmless while nothing looked, and
// the moment something did it failed sixteen tests at once: the fixtures were
// describing a call this product now refuses to make.
//
// Each of these is the shortest byte string that carries its format's
// signature. They are not valid files and are not meant to be: what a wire part
// needs from a fixture is that the thing claiming to be a PNG opens like one.

import "encoding/base64"

// pngSample is the 8-byte PNG signature.
var pngSample = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// jpegSample is the SOI marker plus the APP0 the sniffer looks for.
var jpegSample = []byte{0xff, 0xd8, 0xff, 0xe0}

// gifSample is the 89a version header.
var gifSample = []byte("GIF89a")

// webpSample is a RIFF container whose form type is WEBP; the four bytes
// between are the file length, which nothing here reads.
var webpSample = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")

// bmpSample is the two-byte BM header; the rest of a BMP header is length and
// offsets nothing here reads.
var bmpSample = []byte("BM\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")

// pdfSample carries the version the header must name — `%PDF` alone is text.
var pdfSample = []byte("%PDF-1.7\n")

// heicSample is an ISO-BMFF `ftyp` box with the HEIC brand: a four-byte box
// length, the box type, then the brand.
var heicSample = []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00")

// The same samples as a wire carries them, computed from the bytes rather than
// written out: changing a sample changes the fixture and the expectation about
// it in one edit.
var (
	pngSampleBase64 = base64.StdEncoding.EncodeToString(pngSample)
	pdfSampleBase64 = base64.StdEncoding.EncodeToString(pdfSample)
)

// pdfSampleNamed is a valid-looking PDF whose bytes differ from every other
// one, for a case about telling two documents apart. Its own slice rather than
// an append onto pdfSample: two appends to one slice can share a backing array,
// and then the second fixture silently overwrites the first.
func pdfSampleNamed(suffix string) []byte {
	return []byte(string(pdfSample) + suffix)
}

// sampleBytesFor is the fixture body for a media type, for a case that loops
// over several. It FAILS on a type it has no sample for rather than returning
// something that would be refused: a table-driven test that quietly fed text to
// half its rows is what this file exists to end.
func sampleBytesFor(mime string) []byte {
	switch mime {
	case mimePNG:
		return pngSample
	case mimeJPEG:
		return jpegSample
	case mimeGIF:
		return gifSample
	case mimeWebP:
		return webpSample
	case mimeHEIC, mimeHEIF:
		return heicSample
	case mimeBMP:
		return bmpSample
	case mimePDF:
		return pdfSample
	default:
		return nil
	}
}
