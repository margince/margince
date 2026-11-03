// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// errUnverified refuses a signed post whatever part of it failed, so the 401
// says nothing about which part that was.
var errUnverified = errors.New("webforms: the submission's signature did not verify")

// errForeignOrigin refuses a browser post from a website not on the form's list.
var errForeignOrigin = errors.New("webforms: the form does not take posts from this website")

// SigningSlug is the endpoint name a server form's signature binds, beside the
// form key, so a signature for one form is never valid on another.
const SigningSlug = "web-form"

// signatureSkew bounds how far a signed timestamp may sit from this clock.
const signatureSkew = 5 * time.Minute

// The headers the builders sign with.
const (
	headerWebflowSignature  = "X-Webflow-Signature"
	headerWebflowTimestamp  = "X-Webflow-Timestamp"
	headerTypeformSignature = "Typeform-Signature"
	headerTallySignature    = "Tally-Signature"
)

// minimumFillTime is the least time a human takes to fill a form. A post
// arriving sooner after the page rendered it was not typed.
const minimumFillTime = 3 * time.Second

// Arrival is one request as the intake reads it: the parts admission and the
// fields come from, and nothing else of the HTTP request.
type Arrival struct {
	FormKey     string
	ContentType string
	Header      http.Header
	Body        []byte
	Now         time.Time
}

// admitBrowser checks where a browser post came from. The Origin header is
// the browser's own statement and a page cannot set it; a post without one
// falls back to the Referer's origin. Neither stops a script that forges both,
// which is what the rate limits and spam checks are for.
func admitBrowser(f Form, a Arrival) error {
	origin := a.Header.Get("Origin")
	if origin == "" {
		ref, err := url.Parse(a.Header.Get("Referer"))
		if err != nil || ref.Host == "" {
			return errForeignOrigin
		}
		origin = ref.Scheme + "://" + ref.Host
	}
	if !slices.Contains(f.AllowedOrigins, strings.ToLower(origin)) {
		return errForeignOrigin
	}
	return nil
}

// AllowsOrigin reports whether a browser form takes posts from origin. The
// CORS answer and the admission ask this same rule.
func (f Form) AllowsOrigin(origin string) bool {
	return f.Sender == SenderBrowser && slices.Contains(f.AllowedOrigins, strings.ToLower(origin))
}

// looksAutomated reports the spam checks a browser post fails: a filled
// honeypot, or a page submitted faster than anyone types.
func looksAutomated(e Enquiry, now time.Time) bool {
	if strings.TrimSpace(e.controls[controlHoneypot]) != "" {
		return true
	}
	rendered, err := strconv.ParseInt(e.controls[controlRendered], 10, 64)
	if err != nil {
		return false
	}
	elapsed := now.Sub(time.UnixMilli(rendered))
	return elapsed >= 0 && elapsed < minimumFillTime
}

// verifyServer checks a server form's signature: the Margince inbound recipe,
// bound to this form's key, inside the freshness window. It answers the nonce,
// which keys the submission so a replay lands nothing.
func verifyServer(secret []byte, a Arrival) (string, error) {
	ts, err := strconv.ParseInt(a.Header.Get(extension.InboundHeaderTimestamp), 10, 64)
	if err != nil || !fresh(time.Unix(ts, 0), a.Now) {
		return "", errUnverified
	}
	nonce := a.Header.Get(extension.InboundHeaderNonce)
	if nonce == "" || !extension.ValidInboundNonce(nonce) {
		return "", errUnverified
	}
	presented, ok := strings.CutPrefix(a.Header.Get(extension.InboundHeaderSignature), "sha256=")
	if !ok {
		return "", errUnverified
	}
	payload := extension.SigningPayload(extension.ScopeInbound, SigningSlug, a.FormKey, time.Unix(ts, 0), nonce, a.Body)
	if !hexMACMatches(secret, payload, presented) {
		return "", errUnverified
	}
	return nonce, nil
}

// verifyBuilder checks the signature header a builder documents for its form
// webhooks. Typeform and Tally sign the body alone and carry no timestamp; a
// replay of one of theirs is caught by the builder's own submission id.
func verifyBuilder(sender string, secret []byte, a Arrival) error {
	switch sender {
	case SenderWebflow:
		ms, err := strconv.ParseInt(a.Header.Get(headerWebflowTimestamp), 10, 64)
		if err != nil || !fresh(time.UnixMilli(ms), a.Now) {
			return errUnverified
		}
		signed := append([]byte(strconv.FormatInt(ms, 10)+":"), a.Body...)
		if hexMACMatches(secret, signed, a.Header.Get(headerWebflowSignature)) {
			return nil
		}
	case SenderTypeform:
		presented, ok := strings.CutPrefix(a.Header.Get(headerTypeformSignature), "sha256=")
		if ok && base64MACMatches(secret, a.Body, presented) {
			return nil
		}
	case SenderTally:
		if base64MACMatches(secret, a.Body, a.Header.Get(headerTallySignature)) {
			return nil
		}
	}
	return errUnverified
}

func fresh(at, now time.Time) bool {
	d := now.Sub(at)
	return d <= signatureSkew && d >= -signatureSkew
}

func mac(secret, message []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(message)
	return h.Sum(nil)
}

// hexMACMatches and base64MACMatches compare in constant time, on the decoded
// bytes of both sides.
func hexMACMatches(secret, message []byte, presented string) bool {
	got, err := hex.DecodeString(presented)
	return err == nil && hmac.Equal(got, mac(secret, message))
}

func base64MACMatches(secret, message []byte, presented string) bool {
	got, err := base64.StdEncoding.DecodeString(presented)
	return err == nil && hmac.Equal(got, mac(secret, message))
}
