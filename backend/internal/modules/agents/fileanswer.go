// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"strings"
	"unicode/utf8"
)

// FileAnswer carries a file's bytes in a JSON answer, as attach takes them. A
// text file comes as text too, so a model reads it back without decoding base64.
func FileAnswer(filename, contentType string, content []byte) (json.RawMessage, error) {
	var text string
	if media, _, err := mime.ParseMediaType(contentType); err == nil && strings.HasPrefix(media, "text/") && utf8.Valid(content) {
		text = string(content)
	}
	return json.Marshal(struct {
		Filename      string `json:"filename,omitempty"`
		ContentType   string `json:"content_type"`
		SizeBytes     int    `json:"size_bytes"`
		ContentBase64 string `json:"content_base64"`
		ContentText   string `json:"content_text,omitempty"`
	}{filename, contentType, len(content), base64.StdEncoding.EncodeToString(content), text})
}
