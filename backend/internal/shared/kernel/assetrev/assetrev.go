// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package assetrev names a stored image revision on the wire: the URL's token and the
// stream's ETag are one digest of the object key.
package assetrev

import (
	"crypto/sha256"
	"fmt"
)

// CacheControl is immutable because a replacement takes a new URL. A day, not a year, so a
// picture shown before access was withdrawn expires.
const CacheControl = "private, max-age=86400, immutable"

// Digest is a short one-way name for the object at key. The namespace versions how stored
// bytes are rendered, so changing that changes every URL.
func Digest(namespace, objectKey string) string {
	digest := sha256.Sum256([]byte(namespace + "\x00" + objectKey))
	return fmt.Sprintf("%x", digest[:6])
}
