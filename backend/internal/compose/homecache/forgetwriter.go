// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import "net/http"

// ForgettingWriter runs forget once, when a success or a 409 is first written.
//
// A 409 (version_skew among them) says the row moved under the version the
// writer read, often by a worker write that never ends their cache. Their
// refetch must read the moved row, or every retry meets the same old version.
// Ending the cache here covers every write that can skew, with no client change.
type ForgettingWriter struct {
	http.ResponseWriter
	forget  func()
	settled bool
}

// NewForgettingWriter wraps w so that forget runs once, as the status is first written.
func NewForgettingWriter(w http.ResponseWriter, forget func()) *ForgettingWriter {
	return &ForgettingWriter{ResponseWriter: w, forget: forget}
}

// WriteHeader runs forget on the first success or conflict status.
func (f *ForgettingWriter) WriteHeader(status int) {
	if !f.settled {
		f.settled = true
		if status >= http.StatusOK && status < http.StatusMultipleChoices || status == http.StatusConflict {
			f.forget()
		}
	}
	f.ResponseWriter.WriteHeader(status)
}

// Write without a header is the 200 net/http sends for it.
func (f *ForgettingWriter) Write(body []byte) (int, error) {
	if !f.settled {
		f.WriteHeader(http.StatusOK)
	}
	return f.ResponseWriter.Write(body)
}

// Unwrap lets http.NewResponseController reach the connection through this layer.
func (f *ForgettingWriter) Unwrap() http.ResponseWriter { return f.ResponseWriter }
