// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// WriteJSONTagged writes body as WriteJSON does, tagged with an ETag of its
// bytes. A request whose If-None-Match already names the tag gets 304 and no
// body, so a client re-reading an unchanged answer downloads nothing. The tag is
// weak because compression changes the bytes. The answer is the reader's own, so
// only their browser keeps it, and asks again before each use.
//
//craft:ignore naked-any the JSON serialization seam: the body is whichever contract response struct the handler owns
func WriteJSONTagged(w http.ResponseWriter, r *http.Request, body any) {
	encoded, err := json.Marshal(withEmptyLists(body))
	if err != nil {
		WriteJSON(w, http.StatusOK, body)
		return
	}
	sum := sha256.Sum256(encoded)
	etag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
	if IfNoneMatchHit(r, etag) {
		WriteNotModified(w, etag, readerOwnCache)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", readerOwnCache)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	//craft:ignore swallowed-errors the status line is already on the wire, so a failed write (the client left) has no channel back
	_, _ = w.Write(append(encoded, '\n'))
}

// readerOwnCache keeps a signed-in reader's answer out of shared caches and has
// their browser check it with the server before each use.
const readerOwnCache = "private, no-cache"
