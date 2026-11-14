// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The installation's object store, lent to a unit that declares
// CapabilityObjects. Each unit gets a key prefix of its own in each workspace
// and names only a flat object inside it. It cannot name another unit's key or
// a core attachment.

import (
	"context"
	"fmt"
	"strings"
)

// MaxObjectBytes is the most one object holds: what an inbound file may hold.
const MaxObjectBytes = MaxInboundFileBytes

// maxObjectNameLen keeps a name well inside a storage key.
const maxObjectNameLen = 128

// ValidateObjectName refuses a name that is empty, long, or holds anything but
// lower-case letters, digits, dot, dash and underscore. It has no separator, so
// a name cannot climb out of the unit's prefix, and it may not start with a dot.
func ValidateObjectName(name string) error {
	if name == "" || len(name) > maxObjectNameLen || strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w: an object name is 1 to %d characters and does not start with a dot", ErrInvalid, maxObjectNameLen)
	}
	for _, r := range name {
		lower, digit := r >= 'a' && r <= 'z', r >= '0' && r <= '9'
		if !lower && !digit && r != '.' && r != '-' && r != '_' {
			return fmt.Errorf("%w: an object name holds lower-case letters, digits, '.', '-' and '_' only", ErrInvalid)
		}
	}
	return nil
}

// Object is one stored object. ContentType is what the bytes say, found by the
// core with SniffContentType, never what the unit claimed.
type Object struct {
	Content     []byte
	ContentType string
}

// Objects is the unit's own corner of the object store, in the calling
// workspace. Every method returns ErrNotDeclared unless the unit declares
// CapabilityObjects.
type Objects interface {
	// Put stores content under name, replacing an object of that name. More than
	// MaxObjectBytes, or a bad name, answers ErrInvalid.
	Put(ctx context.Context, name string, content []byte) error

	// Get reads the object, or answers ErrNotFound.
	Get(ctx context.Context, name string) (Object, error)

	// Delete removes the object. Deleting one that is not there is not an error.
	Delete(ctx context.Context, name string) error
}
