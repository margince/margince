// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signed link an erased subject confirms to be stored again. Nothing is
// stored before the click, so the link carries all the request said. The HMAC
// mechanics mirror oidcloginstate.go. The key is its own, derived from the root
// key under returnLinkKeyLabel. The type field keeps another signer's token
// from parsing as this one.
//
// The link works once because it names the erasure it was mailed for:
// LiftEmailSuppressionTx refuses it once that erasure is lifted or replaced.

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
	returnLinkTyp      = "erasure-return"
	returnLinkKeyLabel = "margince erasure return link v1"
	// returnLinkLifetime is how long a mailed link works. The mail says so.
	returnLinkLifetime = 24 * time.Hour
)

// returnRequest is what the subject asked for on the booking page, and the
// erasure the link was mailed for.
type returnRequest struct {
	Slug          string
	Name          string
	Email         string
	PurposeID     ids.UUID
	PolicyVersion string
	Wording       string
	ErasedAt      time.Time
}

type wireReturnRequest struct {
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

type returnLinkSigner struct{ key []byte }

// errNoReturnKey is a signer with no key: the installation has no root key, so
// it mails no link and accepts none.
var errNoReturnKey = errors.New("erasure return link: no key")

func newReturnLinkSigner(key []byte) returnLinkSigner {
	if len(key) < minStateKeyLen {
		return returnLinkSigner{}
	}
	return returnLinkSigner{key: key}
}

func (s returnLinkSigner) sign(req returnRequest, exp time.Time) (string, error) {
	if s.key == nil {
		return "", errNoReturnKey
	}
	payload, err := json.Marshal(wireReturnRequest{
		Typ: returnLinkTyp, S: req.Slug, N: req.Name, E: req.Email, P: req.PurposeID.String(),
		PV: req.PolicyVersion, W: req.Wording, EA: req.ErasedAt.UnixMicro(), Exp: exp.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("erasure return link: %w", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc)), nil
}

func (s returnLinkSigner) verify(token string, now time.Time) (returnRequest, error) {
	if s.key == nil {
		return returnRequest{}, errNoReturnKey
	}
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return returnRequest{}, errors.New("erasure return link: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return returnRequest{}, fmt.Errorf("erasure return link: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return returnRequest{}, errors.New("erasure return link: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return returnRequest{}, fmt.Errorf("erasure return link: bad payload encoding: %w", err)
	}
	var w wireReturnRequest
	if err := json.Unmarshal(payload, &w); err != nil {
		return returnRequest{}, fmt.Errorf("erasure return link: bad payload: %w", err)
	}
	if w.Typ != returnLinkTyp {
		return returnRequest{}, errors.New("erasure return link: wrong token type")
	}
	if now.Unix() > w.Exp {
		return returnRequest{}, errors.New("erasure return link: expired")
	}
	purpose, err := ids.Parse(w.P)
	if err != nil {
		return returnRequest{}, fmt.Errorf("erasure return link: bad purpose: %w", err)
	}
	return returnRequest{
		Slug: w.S, Name: w.N, Email: w.E, PurposeID: purpose, PolicyVersion: w.PV, Wording: w.W,
		ErasedAt: time.UnixMicro(w.EA),
	}, nil
}

func (s returnLinkSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}
