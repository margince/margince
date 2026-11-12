// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package googlejwt reads a Google service-account key file and signs the
// RFC 7523 assertion Google's token endpoint exchanges for an access token.
// Vertex signs with no subject; a mailbox read through domain-wide delegation
// names the mailbox as the subject.
package googlejwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TokenURI is the one token endpoint an assertion is ever posted to. A key
// naming any other is refused: the signed assertion is a bearer credential.
const TokenURI = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: an endpoint address, not a credential

// JWTBearerGrant is the RFC 7523 grant type the assertion is posted under.
const JWTBearerGrant = "urn:ietf:params:oauth:grant-type:jwt-bearer" //nolint:gosec // G101: an OAuth grant type name, not a credential

// ErrInvalidKey marks a key file that cannot be used. Its messages name the
// field at fault and never the value.
var ErrInvalidKey = errors.New("invalid service account key")

// ServiceAccount is the part of a Google service-account key file a signer uses.
type ServiceAccount struct {
	ClientEmail string
	ProjectID   string
	key         *rsa.PrivateKey
}

// ParseServiceAccount reads a key file as Google issues it. The project id is
// returned unchecked, because only some callers splice it into a path.
func ParseServiceAccount(keyFile string) (ServiceAccount, error) {
	var file struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(keyFile), &file); err != nil {
		return ServiceAccount{}, fmt.Errorf("%w: the key file is not a JSON object", ErrInvalidKey)
	}
	switch {
	case file.Type != "service_account":
		return ServiceAccount{}, fmt.Errorf("%w: type must be service_account", ErrInvalidKey)
	case !strings.Contains(file.ClientEmail, "@"):
		return ServiceAccount{}, fmt.Errorf("%w: client_email is missing or not an address", ErrInvalidKey)
	case file.TokenURI != "" && file.TokenURI != TokenURI:
		return ServiceAccount{}, fmt.Errorf("%w: token_uri must be %s", ErrInvalidKey, TokenURI)
	}
	key, err := parseRSAPrivateKey(file.PrivateKey)
	if err != nil {
		return ServiceAccount{}, err
	}
	return ServiceAccount{ClientEmail: file.ClientEmail, ProjectID: file.ProjectID, key: key}, nil
}

// parseRSAPrivateKey reads a PEM key in either encoding Google has issued.
// The parser's own error is dropped: it describes the bytes it was given.
func parseRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	unreadable := fmt.Errorf("%w: private_key is not a PEM-encoded RSA key", ErrInvalidKey)
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, unreadable
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, unreadable
		}
		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, unreadable
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, unreadable
		}
		return key, nil
	default:
		return nil, unreadable
	}
}

// Claims are the parts of one assertion that differ between callers.
type Claims struct {
	Scope string
	// Subject is the user the service account acts for under domain-wide
	// delegation. Empty means the service account acts as itself.
	Subject  string
	IssuedAt time.Time
	TTL      time.Duration
}

// Assertion is the RS256-signed JWT the token endpoint exchanges for an
// access token.
func (a ServiceAccount) Assertion(c Claims) (string, error) {
	if a.key == nil {
		return "", fmt.Errorf("%w: no private key was parsed", ErrInvalidKey)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(struct {
		Issuer   string `json:"iss"`
		Subject  string `json:"sub,omitempty"`
		Audience string `json:"aud"`
		Scope    string `json:"scope"`
		IssuedAt int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}{a.ClientEmail, c.Subject, TokenURI, c.Scope, c.IssuedAt.Unix(), c.IssuedAt.Add(c.TTL).Unix()})
	if err != nil {
		return "", fmt.Errorf("googlejwt: encode assertion claims: %w", err)
	}
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, digest[:]) // NOSONAR(go:S5542) a signature, not encryption: Google's jwt-bearer grant accepts only RS256
	if err != nil {
		return "", errors.New("googlejwt: signing the token assertion failed")
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
