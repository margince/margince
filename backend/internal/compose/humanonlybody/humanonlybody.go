// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package humanonlybody holds the agent gate to the body members a contract
// marks x-human-only.
package humanonlybody

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Refused refuses a body that sets one of members, and reports whether it
// answered. The tool schema already leaves such a member out; this holds it on
// both doors, since a route verb forwards the body it was given. A body over
// limit, or one it cannot read as an object, passes on whole to the handler,
// which refuses it on its own terms.
func Refused(w http.ResponseWriter, r *http.Request, members []string, limit int64) bool {
	if len(members) == 0 {
		return false
	}
	head, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		httperr.Write(w, r, err)
		return true
	}
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(head), r.Body))
	var set map[string]json.RawMessage
	if int64(len(head)) > limit || json.Unmarshal(head, &set) != nil {
		return false
	}
	for _, name := range members {
		if _, ok := set[name]; ok {
			httperr.Write(w, r, fmt.Errorf("only a human may set %s: %w", name, apperrors.ErrPermissionDenied))
			return true
		}
	}
	return false
}
