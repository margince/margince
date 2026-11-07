// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// The raw field gives back the provider's bytes however Gmail pads or escapes
// the text, and refuses text that is not web-safe base64.
func TestTheRawFieldDecodesTheMessageGmailSent(t *testing.T) {
	message := []byte("From: a@b.example\r\nSubject: ~?>\r\n\r\nbody ÿ\r\n")
	padded := base64.URLEncoding.EncodeToString(message)
	unpadded := base64.RawURLEncoding.EncodeToString(message)
	escaped, err := json.Marshal(padded)
	if err != nil {
		t.Fatal(err)
	}
	escaped = bytes.Replace(escaped, []byte("="), []byte(`\u003d`), -1)
	for name, field := range map[string]string{
		"padded":   `"` + padded + `"`,
		"unpadded": `"` + unpadded + `"`,
		"escaped":  string(escaped),
	} {
		var out struct {
			Raw webSafeRFC822 `json:"raw"`
		}
		if err := json.Unmarshal([]byte(`{"raw":`+field+`}`), &out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out.Raw, message) {
			t.Errorf("%s: decoded %q, want %q", name, out.Raw, message)
		}
	}
	// Every other spelling decodes as the old string decode did, or fails
	// where it failed.
	for _, text := range []string{
		unpadded[:8] + "=" + unpadded[8:],
		unpadded[:8] + `\r\n` + unpadded[8:],
		unpadded[:8] + " " + unpadded[8:],
		"not base64!",
	} {
		var plain string
		if err := json.Unmarshal([]byte(`"`+text+`"`), &plain); err != nil {
			t.Fatal(err)
		}
		want, wantErr := base64.RawURLEncoding.DecodeString(strings.TrimRight(plain, "="))
		var out struct {
			Raw webSafeRFC822 `json:"raw"`
		}
		err := json.Unmarshal([]byte(`{"raw":"`+text+`"}`), &out)
		if (err != nil) != (wantErr != nil) || (err == nil && !bytes.Equal(out.Raw, want)) {
			t.Errorf("%q: decoded %q (err %v), the string decode gives %q (err %v)", text, out.Raw, err, want, wantErr)
		}
	}
}
