// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// webSafeRFC822 is the raw field of a message: the RFC822 in web-safe (URL)
// base64, padding optional. It decodes from the response bytes, because a JSON
// string of it first held a large message a third time.
type webSafeRFC822 []byte

// UnmarshalJSON decodes the field. A string with escapes, which Gmail does not
// send, takes the ordinary string decode first.
func (m *webSafeRFC822) UnmarshalJSON(data []byte) error {
	var encoded []byte
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' || bytes.IndexByte(data, '\\') >= 0 {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		encoded = []byte(text)
	} else {
		encoded = data[1 : len(data)-1]
	}
	encoded = bytes.TrimRight(encoded, "=")
	decoded := make([]byte, base64.RawURLEncoding.DecodedLen(len(encoded)))
	n, err := base64.RawURLEncoding.Decode(decoded, encoded)
	if err != nil {
		return fmt.Errorf("gmail: decoding the raw message: %w", err)
	}
	*m = decoded[:n]
	return nil
}
