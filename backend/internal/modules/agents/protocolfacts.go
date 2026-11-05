// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// requestFacts is what one tools/call said about its client. Only a
// ProtocolOnly tool is handed it, so no other tool can vary on the caller's software.
type requestFacts struct {
	modern  bool
	version string
	// elicit is the framing's own reading of the declared capabilities.
	elicit       bool
	capabilities json.RawMessage
	clientInfo   json.RawMessage
}

type requestFactsKey struct{}

func withRequestFacts(ctx context.Context, facts requestFacts) context.Context {
	return context.WithValue(ctx, requestFactsKey{}, facts)
}

func requestFactsOf(ctx context.Context) requestFacts {
	facts, _ := ctx.Value(requestFactsKey{}).(requestFacts)
	return facts
}

func (fr framing) requestFacts() requestFacts {
	return requestFacts{
		modern: fr.modern, version: fr.version, elicit: fr.elicit,
		capabilities: fr.capabilities, clientInfo: fr.clientInfo,
	}
}

// noteHiddenCall logs the traffic a hidden name still carries, which decides
// when it can go. A name the gate withheld is not logged, so the log does not
// tell an operator that a withheld tool exists either.
func (s *Dispatcher) noteHiddenCall(spec mcp.ToolSpec, registered bool, err error) {
	if _, withheld := errors.AsType[*UnknownToolError](err); registered && spec.Unlisted && !withheld {
		s.log.Info("mcp: a hidden tool was called by its own name", "tool", spec.Name)
	}
}
