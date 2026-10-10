// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Registry-served routes: contract operations whose answer is an agent tool's.
// Each handler maps the request onto the tool's arguments and calls the same
// Registry.Invoke the MCP door calls. So a passport gets one admission, one
// charge and one answer on either door. The operation's x-mcp-tool says
// `served_by: registry`, which makes agentGate leave admission to Invoke.

import (
	"encoding/json"
	"fmt"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// ListSlippingDeals serves whats_slipping_this_week.
func (s Server) ListSlippingDeals(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSlippingDealsParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "whats_slipping_this_week", params)
}

// serveTool runs one tool for a REST request and writes the tool's payload.
//
// args becomes the tool's argument object as JSON. Name the parameters after
// the tool's arguments and a generated Params struct passes straight through.
// Each optional field is tagged omitempty, so an omitted parameter stays an
// omitted argument and the tool applies its own default.
//
//craft:ignore naked-any json.Marshal's own input: the args are whichever Params or body type the contract generated
func serveTool(w http.ResponseWriter, r *http.Request, invoke toolInvoker, tool string, args any) {
	in, err := json.Marshal(args)
	if err != nil {
		httperr.Write(w, r, fmt.Errorf("compose: encoding the arguments of %s: %w", tool, err))
		return
	}
	sealed, err := invoke(r.Context(), tool, in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeToolPayload(w, r, sealed)
}

// writeToolPayload answers a REST call with the envelope's `data`, the payload
// an operation's 200 schema declares. The trace id moves to a header. The MCP
// door answers the whole envelope, so its `data` and this body are one JSON.
func writeToolPayload(w http.ResponseWriter, r *http.Request, sealed json.RawMessage) {
	payload, trace, err := unwrapToolEnvelope(sealed)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if trace != "" {
		w.Header().Set(toolTraceHeader, trace)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	//craft:ignore swallowed-errors WriteHeader already committed the response — nothing can report a write failure to the client anymore
	_, _ = w.Write(payload)
}
