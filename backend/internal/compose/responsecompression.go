// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

func compressAPIJSON(next http.Handler) http.Handler {
	compressed := middleware.Compress(5, "application/json", "application/problem+json")(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsAPIGzip(r.Header.Values("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		// chi matches coding names without checking weights. Give it only the
		// accepted coding, so a refused offer never enables compression.
		request := r.Clone(r.Context())
		request.Header.Set("Accept-Encoding", "gzip")
		compressed.ServeHTTP(w, request)
	})
}

func acceptsAPIGzip(headers []string) bool {
	qualities := make(map[string]float64)
	for _, line := range headers {
		for offer := range strings.SplitSeq(line, ",") {
			coding, params, err := mime.ParseMediaType(offer)
			if err != nil {
				continue
			}
			quality := 1.0
			if raw, weighted := params["q"]; weighted {
				parsed, err := strconv.ParseFloat(raw, 64)
				if err != nil || !(parsed >= 0 && parsed <= 1) {
					quality = 0
				} else {
					quality = parsed
				}
			}
			qualities[coding] = quality
		}
	}
	quality, explicit := qualities["gzip"]
	if !explicit {
		quality = qualities["*"]
	}
	if quality <= 0 {
		return false
	}
	identity, explicitIdentity := qualities["identity"]
	return !explicitIdentity || identity <= quality
}
