// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireSentTellsAnOmittedKeyFromANull(t *testing.T) {
	for body, refused := range map[string]bool{
		`{}`:                  true,
		`{"other":1}`:         true,
		`{"expires_at":null}`: false,
		`{"expires_at":"x"}`:  false,
	} {
		r := httptest.NewRequest("PUT", "/v1/things", strings.NewReader(body))
		var into map[string]any
		if !Decode(httptest.NewRecorder(), r, &into) {
			t.Fatalf("%s: decode refused", body)
		}
		err := RequireSent(r, "expires_at", "send expires_at")
		if (err != nil) != refused {
			t.Errorf("%s: RequireSent = %v, want refused=%v", body, err, refused)
		}
	}
}
