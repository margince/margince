// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// refusedHumanOnlyMember refuses an agent body that sets a member the contract
// marks x-human-only, and reports whether it answered. The tool schema already
// leaves such a member out; this holds it on both doors, since the route verb
// forwards the body it was given. A body it cannot read as an object passes on
// to the handler, which refuses it on its own terms.
func refusedHumanOnlyMember(w http.ResponseWriter, r *http.Request) bool {
	shape, ok := routeVerbShapes[r.Method+" "+chi.RouteContext(r.Context()).RoutePattern()]
	if !ok || len(shape.HumanOnly) == 0 {
		return false
	}
	body, err := readGatedBody(w, r, maxGatedBody)
	if err != nil {
		httperr.Write(w, r, err)
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var members map[string]json.RawMessage
	if json.Unmarshal(body, &members) != nil {
		return false
	}
	for _, name := range shape.HumanOnly {
		if _, set := members[name]; set {
			httperr.Write(w, r, fmt.Errorf("only a human may set %s: %w", name, apperrors.ErrPermissionDenied))
			return true
		}
	}
	return false
}
