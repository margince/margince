// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SignatureHeader carries `mac=<hex>` on every webhook delivery.
const SignatureHeader = "X-ZEvent-Signature"

// MaxBodyBytes bounds a webhook body read. Zalo events are small JSON
// documents; the cap stops an unauthenticated sender from exhausting memory.
const MaxBodyBytes = 1 << 20

// ReadBody reads a webhook body under MaxBodyBytes.
func ReadBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("zalo: reading the webhook body: %w", ErrMalformedEvent)
	}
	if len(body) > MaxBodyBytes {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// macInput is the one place the signed byte string is spelled: Zalo hashes
// app_id + raw body + timestamp + OA secret key with plain SHA-256.
func macInput(appID string, body []byte, timestamp, secret string) []byte {
	in := make([]byte, 0, len(appID)+len(body)+len(timestamp)+len(secret))
	in = append(in, appID...)
	in = append(in, body...)
	in = append(in, timestamp...)
	in = append(in, secret...)
	return in
}

// VerifySignature checks the X-ZEvent-Signature header value against body
// under the OA secret key. Only app_id and timestamp are read from the body
// before the check passes; they are the MAC's own inputs.
func VerifySignature(secret, header string, body []byte) error {
	header = strings.TrimSpace(header)
	if header == "" {
		return ErrSignatureMissing
	}
	got, ok := strings.CutPrefix(header, "mac=")
	if !ok || secret == "" {
		return ErrSignatureInvalid
	}
	given, err := hex.DecodeString(strings.TrimSpace(got))
	if err != nil {
		return ErrSignatureInvalid
	}
	var env struct {
		AppID     idString `json:"app_id"`
		Timestamp idString `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ErrSignatureInvalid
	}
	want := sha256.Sum256(macInput(string(env.AppID), body, string(env.Timestamp), secret))
	if !hmac.Equal(given, want[:]) {
		return ErrSignatureInvalid
	}
	return nil
}
