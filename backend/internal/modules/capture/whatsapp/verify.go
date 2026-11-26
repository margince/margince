// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package whatsapp is the WhatsApp Business (Meta Cloud API) protocol layer.
// It verifies webhooks, decodes their payloads into typed events, and makes the
// Graph API calls for media download and text send. It owns no storage and files no
// messages; the capture path that consumes its events lives elsewhere.
//
// Coexistence matters for the shape of the events. The customer's messages
// arrive in the `messages` webhook field, and what staff type in the WhatsApp
// Business app arrives in `smb_message_echoes`. Both decode to one Message with
// a Direction, so the caller captures one conversation from both sides.
package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MaxWebhookBodyBytes bounds one webhook POST. Meta batches several changes in
// one call but a body this large is not one of its.
const MaxWebhookBodyBytes = 1 << 20

// ErrBadVerifyToken marks a subscription challenge that did not carry the
// configured verify token, or came without one configured.
var ErrBadVerifyToken = errors.New("whatsapp: webhook verify token mismatch")

// ErrBadSignature marks a POST whose X-Hub-Signature-256 is missing,
// malformed or not the HMAC of the body under the app secret.
var ErrBadSignature = errors.New("whatsapp: webhook signature invalid")

// ErrBodyTooLarge marks a POST body over MaxWebhookBodyBytes.
var ErrBodyTooLarge = errors.New("whatsapp: webhook body too large")

const signaturePrefix = "sha256="

// VerifyChallenge answers Meta's GET subscription handshake: it returns the
// hub.challenge to echo when hub.mode is subscribe and hub.verify_token equals
// the configured token. The comparison is constant-time. An empty configured
// token never matches, so an unconfigured install cannot be subscribed by a
// request that sends an empty token.
func VerifyChallenge(query url.Values, verifyToken string) (string, error) {
	got := query.Get("hub.verify_token")
	if verifyToken == "" || query.Get("hub.mode") != "subscribe" ||
		subtle.ConstantTimeCompare([]byte(got), []byte(verifyToken)) != 1 {
		return "", ErrBadVerifyToken
	}
	return query.Get("hub.challenge"), nil
}

// VerifySignature checks header (`sha256=<hex>`) against the HMAC-SHA256 of
// body under appSecret. An empty secret fails: a missing configuration must
// not turn into an HMAC anyone can compute.
func VerifySignature(appSecret string, body []byte, header string) error {
	hexSum, ok := strings.CutPrefix(header, signaturePrefix)
	if !ok || appSecret == "" {
		return ErrBadSignature
	}
	got, err := hex.DecodeString(hexSum)
	if err != nil || len(got) != sha256.Size {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	// hash.Hash.Write never returns an error.
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}

// ReadBody reads a webhook POST body under the size limit. The caller checks the
// signature against the secrets it holds before it decodes anything.
func ReadBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes))
	if err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			return nil, ErrBodyTooLarge
		}
		return nil, err
	}
	return body, nil
}
