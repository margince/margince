// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package picturerefusal answers an uploaded picture the decoder would not read.
// The avatar and company-logo routes share it, so each refusal has one code.
package picturerefusal

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

// Busy answers an upload whose decode found no free slot in time. The picture
// itself was not judged, so the reader is told to send the same file again.
var Busy = &httperr.DetailedError{
	Status: http.StatusServiceUnavailable, Code: "pictures_busy", RetryAfterSeconds: 10,
	Detail: "the server is reading other pictures right now; send the same file again in a few seconds",
}

// TooManyPixels answers a readable picture past the decode bound, so its sender learns
// to scale it down rather than to change its format.
var TooManyPixels = &httperr.DetailedError{
	Status: http.StatusUnprocessableEntity, Code: "too_many_pixels",
	Detail: fmt.Sprintf("the picture has more than %d pixels; scale it down and upload it again", imagenorm.MaxSourcePixels),
}

// WriteMarkDecode names what a mark the decoder refused would need.
func WriteMarkDecode(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, imagenorm.ErrSVGTextNotOutlined):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: "svg_text_not_outlined",
			Detail: "the SVG draws its words as text, which this server cannot render; convert the text to outlines in your design tool, or upload a PNG",
		})
	case errors.Is(err, imagenorm.ErrSVGNotDrawable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: "svg_not_drawable",
			Detail: "the SVG has parts this server cannot draw, such as embedded images; export it as a PNG",
		})
	case errors.Is(err, imagenorm.ErrDecodeBusy):
		httperr.Write(w, r, Busy)
	case errors.Is(err, imagenorm.ErrTooManyPixels):
		httperr.Write(w, r, TooManyPixels)
	case errors.Is(err, imagenorm.ErrUnsupported):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type",
			Detail: "the upload is not an image this server can read; PNG, JPEG, GIF, WebP, ICO and SVG work",
		})
	default:
		httperr.Write(w, r, err)
	}
}
