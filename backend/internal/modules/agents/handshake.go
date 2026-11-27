// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The opening call, and what the surface says about itself in it.
//
// Apart from the dispatcher because the guidance is PROSE, edited as prose, for
// the reason warnings.go is its own file: these sentences are instructions about
// a conclusion not to draw, written for a reader that will otherwise draw it,
// and maintaining them is a different job from the plumbing that carries them.

import (
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

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
		"instructions": s.instructions(),
	}, nil
}

// instructions is the guidance as THIS composition can honour it.
//
// The approvals sentence names a tool BY NAME, and RegisterApprovalTools
// registers nothing without an inbox, so the sentence is served only beside it.
func (s *Dispatcher) instructions() string {
	guidance := orientation + toolNamesRule + s.startingTools() + surfaceInstructions
	if _, served := s.registry.Spec(listApprovalsToolName); !served {
		return guidance
	}
	return guidance + " " + approvalsClaimRule
}

// orientation says what this server is, ahead of the rules. A host that loads
// tools on demand reads these instructions before any tool's own text.
const orientation = "Margince is a CRM: contacts, companies, deals, leads, projects and the " +
	"activity on them. When an answer names a record, give the reader its evidence web_url so " +
	"they can open it. Money comes as <name>_minor in minor units with a readable <name>_display " +
	"beside it; quote the display. "

// toolNamesRule comes before the first tool this text names. The server cannot
// know the prefix a client adds to a tool's name, so it names tools bare and says so.
const toolNamesRule = "This server names its tools by their own names, here, in every tool " +
	"description and in its answers. Your client may list each tool under a longer name with a " +
	"prefix of its own; call a tool by the name your client lists. "

// startingTools names the first tools to reach for, only those this composition
// serves, for the same reason the approvals sentence is conditional.
func (s *Dispatcher) startingTools() string {
	var served []string
	for _, name := range []string{readAttentionToolName, findRecordsToolName, readContextToolName, "read_record"} {
		if _, ok := s.registry.Spec(name); ok {
			served = append(served, name)
		}
	}
	if len(served) == 0 {
		return ""
	}
	return "Start with " + strings.Join(served, ", ") + ". "
}

// surfaceInstructions is the guidance for the model on the other side of the
// client, kept to what is true of every tool. governanceDefaults is the half of
// each description DescribeForClient leaves out, so the two read as one text.
//
// One string for both eras. `initialize` and `server/discover` are the same
// claim to two generations of client, and two spellings of one claim is how a
// client ends up told different things by the same server — the reason
// `capabilities` is shared already.
const surfaceInstructions = "A governed CRM tool surface. Every call re-authenticates and is bounded by " +
	"the granting human's own permissions, so a tool may refuse a record this passport cannot " +
	"reach. Tools that a human must approve say so in their own description; calling one " +
	"stages the effect for review rather than performing it. " + governanceDefaults + " " +
	mcp.UntrustedContentRule + " " +
	mcp.ConflictingSourcesRule + " " +
	mcp.LocalTimeRule + " " + mcp.OwnershipRule

// approvalsClaimRule bounds a claim about the queue without ordering a call.
// An automation may stage a draft after a write returns, so an answer that has
// not just read the queue cannot know it is empty.
const approvalsClaimRule = "Say that nothing is waiting for approval only when the user asked and " +
	"you have just read list_approvals."

// identity is the serverInfo both framings report: the handshake era in its
// initialize result, the modern era in every result's _meta.
// iconSizeAny is the sizes value of an icon that scales, as the SVG does.
const iconSizeAny = "any"

func (s *Dispatcher) identity() map[string]any {
	info := map[string]any{
		fieldName: s.name, "version": s.version,
		fieldTitle:       serverTitle,
		fieldDescription: serverDescription,
	}
	if s.publicOrigin != "" {
		info["websiteUrl"] = s.publicOrigin
		info["icons"] = []map[string]any{
			{"src": s.publicOrigin + "/icon.svg", "mimeType": "image/svg+xml", "sizes": []string{iconSizeAny}},
			{"src": s.publicOrigin + "/web-app-manifest-512x512.png", "mimeType": "image/png", "sizes": []string{"512x512"}},
		}
	}
	return info
}

// The name and line a host shows in its connector list.
const (
	serverTitle       = "Margince"
	serverDescription = "Your CRM in the conversation: deals, contacts, meetings, the forecast and what needs attention."
)

// eraOwned answers the calls that exist in one framing only, and reports
// whether the method was one of them at all.
//
// Each era's opening call is here because answering the other era's would
// mislead a client about which kind of server it found. That is the
// question those two calls exist to settle. ping is here because the 2026-07-28
// revision removed it with the handshake it kept alive.
func (s *Dispatcher) eraOwned(req rpcRequest, fr framing) (rpcResponse, bool) {
	resp := rpcResponse{JSONRPC: jsonRPCVersion, ID: req.ID}
	switch req.Method {
	case methodInitialize:
		if fr.modern {
			return methodNotFound(req), true
		}
		if result, rpcErr := s.initialize(req.Params); rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
	case methodDiscover:
		if !fr.modern {
			return methodNotFound(req), true
		}
		resp.Result = s.discover()
	case methodPing:
		if fr.modern {
			return methodNotFound(req), true
		}
		resp.Result = map[string]any{}
	default:
		return rpcResponse{}, false
	}
	return resp, true
}
