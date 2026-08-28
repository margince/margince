// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/http/httptest"
	"testing"
)

// failingReader answers every Read with an error that is not
// *http.MaxBytesError — the shape a client that vanished mid-body or a
// malformed chunked encoding produces, as opposed to the body simply being
// larger than the configured limit.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset by peer") }
func (failingReader) Close() error             { return nil }

// A read failure that is not an over-cap must answer 400, not the 413 an
// honest sender would read as "shrink the body" (it would not help) nor the
// opaque 401 reserved for authentication (a truncated upload is not a
// signature question).
func TestReadInboundBodyAnswersBadRequestForANonSizeReadFailure(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhooks/ext/u/capture", failingReader{})
	w := httptest.NewRecorder()

	_, ok := readInboundBody(w, r, 64<<10)

	if ok {
		t.Fatal("readInboundBody reported the body usable despite the read failing")
	}
	if w.Code != 400 {
		t.Fatalf("a non-size read failure answered %d, want 400", w.Code)
	}
}
