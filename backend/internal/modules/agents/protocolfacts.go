// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
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
