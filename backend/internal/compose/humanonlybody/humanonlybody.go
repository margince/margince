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
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Marks are one route's body markers: the members a human alone sets, and the
// values an agent may send in a member.
type Marks struct {
	HumanOnly   []string
	AgentValues map[string][]string
}

// Refused refuses a body that sets a human-only member. It also refuses one that
// sends a value the agent may not send, and reports whether it answered. The tool
// schema already leaves such a member out or narrows it. This holds it on both
// doors, since a route verb forwards the body it was given. A body over limit,
// or one it cannot read as an object, passes on whole to the handler, which
// refuses it on its own terms.
func Refused(w http.ResponseWriter, r *http.Request, marks Marks, limit int64) bool {
	if len(marks.HumanOnly) == 0 && len(marks.AgentValues) == 0 {
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
	for key, value := range set {
		if i := slices.IndexFunc(marks.HumanOnly, func(name string) bool { return strings.EqualFold(key, name) }); i >= 0 {
			httperr.Write(w, r, fmt.Errorf("only a human may set %s: %w", marks.HumanOnly[i], apperrors.ErrPermissionDenied))
			return true
		}
		if refusal := valueRefusal(marks.AgentValues, key, value); refusal != nil {
			httperr.Write(w, r, refusal)
			return true
		}
	}
	return false
}

// codeAgentValue is the field code of a value an agent may not send.
const codeAgentValue = "agent_value_refused"

// valueRefusal is the answer to an agent sending member a value it may not, or nil.
// It is a 422 on the member, not a 403: the agent fixes the argument, and no
// grant it could ask for would admit the value.
func valueRefusal(agentValues map[string][]string, key string, value json.RawMessage) error {
	for member, allowed := range agentValues {
		if !strings.EqualFold(key, member) {
			continue
		}
		var word string
		if json.Unmarshal(value, &word) == nil && slices.Contains(allowed, word) {
			return nil
		}
		return httperr.Validation(member, codeAgentValue, fmt.Sprintf(
			"an agent may send %[1]s only as %[2]s, so nothing was changed; call again with %[1]s %[2]s, or leave %[1]s to the user",
			member, strings.Join(allowed, " or ")))
	}
	return nil
}
