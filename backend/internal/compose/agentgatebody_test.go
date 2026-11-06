// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("the connection dropped") }

// A body over the limit names the limit. One that merely cannot be read says so, and
// is not told to shrink.
func TestTheAgentGateTellsAnOverLimitBodyFromAnUnreadableOne(t *testing.T) {
	rec := httptest.NewRecorder()
	over := httptest.NewRequest(http.MethodPost, "/v1/contacts/vcard-import", strings.NewReader(strings.Repeat("x", 2_000_001)))
	_, err := readGatedBody(rec, over, 2_000_000)
	detailed, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || detailed.Status != http.StatusRequestEntityTooLarge || !httperr.BodyTooLarge(err) || !strings.Contains(detailed.Detail, "2 MB") {
		t.Errorf("a body over the limit answered %+v", err)
	}
	at := httptest.NewRequest(http.MethodPost, "/v1/contacts/vcard-import", strings.NewReader(strings.Repeat("x", 2_000_000)))
	if body, err := readGatedBody(rec, at, 2_000_000); err != nil || len(body) != 2_000_000 {
		t.Errorf("a body at the limit answered %d bytes, %v", len(body), err)
	}
	broken := httptest.NewRequest(http.MethodPost, "/v1/contacts/vcard-import", brokenBody{})
	_, err = readGatedBody(rec, broken, 2_000_000)
	if httperr.BodyTooLarge(err) || err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("an unreadable body answered %v", err)
	}
}
