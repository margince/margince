// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package returnlink signs the link an erased subject confirms to be stored
// again. Nothing is stored before the click, so the link carries all the
// request said. The HMAC mechanics mirror compose/oidcloginstate.go. The key is
// its own, derived from the root key under KeyLabel. The type field keeps
// another signer's token from parsing as this one.
//
// The link works once because it names the erasure it was mailed for:
// LiftEmailSuppressionTx refuses it once that erasure is lifted or replaced.
package returnlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	linkTyp = "erasure-return"
	// KeyLabel derives the signing key from the root key.
	KeyLabel = "margince erasure return link v1"
	// Lifetime is how long a mailed link works. The mail says so.
	Lifetime = 24 * time.Hour
)

// Request is what the subject asked for on the booking page, and the
// erasure the link was mailed for.
type Request struct {
	Slug          string
	Name          string
	Email         string
	PurposeID     ids.UUID
	PolicyVersion string
	Wording       string
	ErasedAt      time.Time
}

type wireRequest struct {
	Typ string `json:"t"`
	S   string `json:"s"`
	N   string `json:"n"`
	E   string `json:"e"`
	P   string `json:"p"`
	PV  string `json:"pv"`
	W   string `json:"w"`
	EA  int64  `json:"ea"`
	Exp int64  `json:"exp"`
}

// Signer signs and verifies return links.
type Signer struct{ key []byte }

// errNoKey is a signer with no key: the installation has no root key, so
// it mails no link and accepts none.
var errNoKey = errors.New("erasure return link: no key")

// minKeyLen is the shortest key a signer accepts.
const minKeyLen = 32

// NewSigner signs with key, or with none when the key is too short.
func NewSigner(key []byte) Signer {
	if len(key) < minKeyLen {
		return Signer{}
	}
	return Signer{key: key}
}

// Sign mints the link for req, working until exp.
func (s Signer) Sign(req Request, exp time.Time) (string, error) {
	if s.key == nil {
		return "", errNoKey
	}
	payload, err := json.Marshal(wireRequest{
		Typ: linkTyp, S: req.Slug, N: req.Name, E: req.Email, P: req.PurposeID.String(),
		PV: req.PolicyVersion, W: req.Wording, EA: req.ErasedAt.UnixMicro(), Exp: exp.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("erasure return link: %w", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc)), nil
}

// Verify answers the request a link carries, or why it does not work.
func (s Signer) Verify(token string, now time.Time) (Request, error) {
	if s.key == nil {
		return Request{}, errNoKey
	}
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return Request{}, errors.New("erasure return link: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return Request{}, fmt.Errorf("erasure return link: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return Request{}, errors.New("erasure return link: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return Request{}, fmt.Errorf("erasure return link: bad payload encoding: %w", err)
	}
	var w wireRequest
	if err := json.Unmarshal(payload, &w); err != nil {
		return Request{}, fmt.Errorf("erasure return link: bad payload: %w", err)
	}
	if w.Typ != linkTyp {
		return Request{}, errors.New("erasure return link: wrong token type")
	}
	if now.Unix() > w.Exp {
		return Request{}, errors.New("erasure return link: expired")
	}
	purpose, err := ids.Parse(w.P)
	if err != nil {
		return Request{}, fmt.Errorf("erasure return link: bad purpose: %w", err)
	}
	return Request{
		Slug: w.S, Name: w.N, Email: w.E, PurposeID: purpose, PolicyVersion: w.PV, Wording: w.W,
		ErasedAt: time.UnixMicro(w.EA),
	}, nil
}

func (s Signer) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}
