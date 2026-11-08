// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A preference link names its preference_token row and carries a MAC over the
// row id under a key derived from the root key. The database holds no part of
// it that works as a link, and every send can hand out the same link again.
const (
	preferenceLinkPrefix = "pref2_"
	preferenceKeyLabel   = "margince preference link v1"
)

// preferenceLinkKey is the key links are signed with, or nil when the
// installation configures no root key.
func preferenceLinkKey() []byte {
	return keyvault.DerivedKey(config.Bound(), preferenceKeyLabel)
}

func preferenceLinkMAC(key []byte, row ids.UUID) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(row[:])
	return mac.Sum(nil)
}

// mintPreferenceLink is the link for row under key.
func mintPreferenceLink(key []byte, row ids.UUID) string {
	return preferenceLinkPrefix + base64.RawURLEncoding.EncodeToString(append(row[:], preferenceLinkMAC(key, row)...))
}

// errUnissuableLink is a live row whose stored link was hashed while a root
// key was set, asked for again by a process that has none.
var errUnissuableLink = errors.New("consent: this recipient's preference link needs the root key it was made under; restore MARGINCE_KEYVAULT_ROOT_KEY")

// issuePreferenceLink is the link a send carries for a row: a signed one when
// a key is configured, and the row's stored link otherwise.
func issuePreferenceLink(row ids.UUID, stored *string) (string, error) {
	if key := preferenceLinkKey(); key != nil {
		return mintPreferenceLink(key, row), nil
	}
	if stored == nil {
		return "", errUnissuableLink
	}
	return *stored, nil
}

// preferenceTokenRowSQL finds, over preference_token aliased pt, the row a
// link names. Both readers of a link ask through it: a signed link by the row
// signedPreferenceRow verified, and an older one by its hash, or as stored
// until the re-hash has reached its row.
const preferenceTokenRowSQL = `(pt.id = @link_row OR (@link_row::uuid IS NULL
	AND (pt.token_hash = @link_hash OR pt.token = @link_token)))`

// signedPreferenceRow is the row a signed link names, nil for an older link.
// ok is false for a signed link whose MAC does not hold under the current key.
func signedPreferenceRow(token string) (row *ids.UUID, ok bool) {
	if !strings.HasPrefix(token, preferenceLinkPrefix) {
		return nil, true
	}
	key := preferenceLinkKey()
	raw, err := base64.RawURLEncoding.DecodeString(token[len(preferenceLinkPrefix):])
	if key == nil || err != nil || len(raw) != len(ids.UUID{})+sha256.Size {
		return nil, false
	}
	var named ids.UUID
	copy(named[:], raw)
	if !hmac.Equal(raw[len(named):], preferenceLinkMAC(key, named)) {
		return nil, false
	}
	return &named, true
}
