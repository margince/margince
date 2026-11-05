// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"io"
	"net/http"
)

// OwnImage is an image this server encoded itself: a company's mark, a seat's
// picture. ETag is empty for an image whose URL names no revision.
type OwnImage struct {
	ContentType  string
	ETag         string
	CacheControl string
	Size         int64
	Body         io.ReadCloser
}

// StreamOwnImage fixes the media type rather than trusting the stored object's. Nothing may
// sniff it into something active, and a document that renders it reaches nothing.
func StreamOwnImage(w http.ResponseWriter, r *http.Request, img OwnImage, logLabel string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", img.CacheControl)
	if img.ETag != "" {
		w.Header().Set("ETag", img.ETag)
	}
	StreamObject(w, r, StreamedObject{
		Download: Download{ContentType: img.ContentType, Inline: true, Size: img.Size},
		Body:     img.Body,
	}, logLabel)
}

// WriteNotModified answers a revalidation that already holds etag, under the
// same freshness a full answer carries.
func WriteNotModified(w http.ResponseWriter, etag, cacheControl string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	w.WriteHeader(http.StatusNotModified)
}
