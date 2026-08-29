// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The opening call, and what the surface says about itself in it.
//
// Apart from the dispatcher because the guidance is PROSE, edited as prose, for
// the reason warnings.go is its own file: these sentences are instructions about
// a conclusion not to draw, written for a reader that will otherwise draw it,
// and maintaining them is a different job from the plumbing that carries them.

import "encoding/json"

// initialize answers the handshake era's opening call: the revision this
// server will speak with THIS client, what it can do, and who it is.
func (s *Dispatcher) initialize(rawParams json.RawMessage) (map[string]any, *rpcError) {
	var params struct {
		//nolint:tagliatelle // protocolVersion is the MCP wire member, camelCase by the protocol
		ProtocolVersion string `json:"protocolVersion"`
	}
	// Params is optional on the wire; only unmarshal when the client sent
	// some, so an omitted field (not malformed JSON) falls through to the
	// negotiator's absent-value default rather than an error.
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			// The decoder's own message names Go types, which is server-side
			// detail an untrusted client has no use for. It is told what to
			// send instead.
			s.log.Warn("mcp: initialize params did not decode", "err", err)
			return nil, &rpcError{
				Code:    codeInvalidParams,
				Message: `invalid params: initialize takes an object whose "protocolVersion" is a string`,
			}
		}
	}
	return map[string]any{
		"protocolVersion": negotiateLegacyVersion(params.ProtocolVersion),
		"capabilities":    s.capabilities(false),
		"serverInfo":      s.identity(),
		// The SAME guidance server/discover hands a modern client. It was on
		// one era only, and the era it was missing from is the one most clients
		// speak — so the surface's own rule about what a write can leave behind
		// reached nobody who needed it.
		"instructions": surfaceInstructions,
	}, nil
}

// surfaceInstructions is the guidance for the model on the other side of the
// client, kept to what is true of EVERY tool: the per-tool text is
// DescribeForClient's, and a second description of the governance here would be
// a second answer to the same question.
//
// One string for both eras. `initialize` and `server/discover` are the same
// claim to two generations of client, and two spellings of one claim is how a
// client ends up told different things by the same server — the reason
// `capabilities` is shared already.
//
// THE LAST SENTENCE IS A REPORTING RULE, and it is here because nothing else
// reaches. An assistant finished a run of correct writes and reported "nothing
// pending approval this time" while a drafted reply sat in the queue: the
// automation that staged it ran in reaction to what the agent had just logged,
// after every one of its calls had returned. No write envelope can carry that —
// the row did not exist when the write answered — so the only thing that
// prevents the false report is the model knowing to look before it makes one.
const surfaceInstructions = "A governed CRM tool surface. Every call re-authenticates and is bounded by " +
	"the granting human's own permissions, so a tool may refuse a record this passport cannot " +
	"reach. Tools that a person must approve say so in their own description; calling one " +
	"stages the effect for review rather than performing it. " +
	"A write can also leave a question for a human WITHOUT saying so in its answer — an " +
	"automation reacting to what you just wrote may draft a reply or stage a change of its own, " +
	"after your call returned. Before telling anyone the work is finished, call list_approvals " +
	"and report what is waiting; it needs no approval of its own."
