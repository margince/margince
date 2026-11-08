// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package keyvault

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/config"
)

// derivedKeyBytes is the length of every key DerivedKey answers.
const derivedKeyBytes = 32

// DerivedKey is the 32-byte key HKDF-SHA256 derives from the root key c holds,
// under label, or nil when c is nil, or its program reads no root key or
// configures none. Each label names one use,
// so no two uses share a key and a new use needs no new secret.
//
// It changes whenever the root key does. Data hashed under it then stops
// matching, so a root key rotation has to re-hash that data.
//
// The setting's Validate refuses a key that does not decode when the
// configuration loads, so a Config holding one cannot reach here.
func DerivedKey(c *config.Config, label string) []byte {
	if c == nil {
		return nil
	}
	if _, registered := c.Registry().Lookup(rootKeySetting.Key()); !registered {
		return nil
	}
	encoded := RootKey(c)
	if encoded == "" {
		return nil
	}
	root, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic(fmt.Sprintf("keyvault: %s passed validation and does not decode", EnvRootKey))
	}
	key, err := hkdf.Key(sha256.New, root, nil, label, derivedKeyBytes)
	if err != nil {
		panic(fmt.Sprintf("keyvault: deriving a %d-byte key cannot fail: %v", derivedKeyBytes, err))
	}
	return key
}
