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

// DerivedKey is the 32-byte key HKDF-SHA256 derives from c's root key under
// label. It is nil when c is nil, or reads or configures no root key. Each
// label names one use, so no two uses share a key.
//
// It changes with the root key, so a root key rotation has to re-hash the data
// hashed under it. The setting's Validate refuses a key that does not decode.
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
