// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/pkg/extension"
)

// maxHookedBody bounds the request body an operation hook reads, the same
// order as a tool's argument document.
const maxHookedBody = 1 << 20

// operationHooks runs every composed unit's hooks on a contract operation
// before core does, in unit order. The operation is named by agentPolicies,
// the generated route table the agent gate reads too.
func operationHooks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opID := agentPolicies[r.Method+" "+chi.RouteContext(r.Context()).RoutePattern()].Op
		units := unitsHooking(opID)
		if opID == "" || len(units) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxHookedBody+1))
		if err != nil || len(body) > maxHookedBody {
			httperr.Write(w, r, fmt.Errorf("%w: the request body is unreadable or over %d bytes", apperrors.ErrInvalidArgument, maxHookedBody))
			return
		}
		op := &extension.Operation{ID: opID, Body: body}
		for _, e := range units {
			if err := runOperationHooks(r, e, op); err != nil {
				httperr.Write(w, r, unitRefusal(err))
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(op.Body))
		r.ContentLength = int64(len(op.Body))
		next.ServeHTTP(w, r)
	})
}

func unitsHooking(opID string) []extension.Extension {
	var units []extension.Extension
	for _, e := range ComposedExtensions() {
		if slices.ContainsFunc(e.OperationHooks, func(h extension.OperationHook) bool { return slices.Contains(h.Operations, opID) }) {
			units = append(units, e)
		}
	}
	return units
}

func runOperationHooks(r *http.Request, e extension.Extension, op *extension.Operation) error {
	rt := runtimeFor(r.Context(), string(e.Name), string(e.Version), "hook/operation/"+op.ID, boundExtensionRuntime())
	defer rt.release()
	for _, hook := range e.OperationHooks {
		if !slices.Contains(hook.Operations, op.ID) {
			continue
		}
		if err := hook.Handle(r.Context(), rt, op); err != nil {
			return err
		}
	}
	return nil
}

// preflightHooks refuses a hook that could never run: one with no handler, no
// entity or action, or an operation the contract does not have.
func preflightHooks(e extension.Extension) error {
	for _, h := range e.WriteHooks {
		if h.Handle == nil || h.Entity == "" || len(h.Actions) == 0 {
			return fmt.Errorf("compose: extension %q declares a write hook on %q with no handler, entity or action", e.Name, h.Entity)
		}
	}
	for _, h := range e.OperationHooks {
		if h.Handle == nil || len(h.Operations) == 0 {
			return fmt.Errorf("compose: extension %q declares an operation hook with no handler or operation", e.Name)
		}
		for _, op := range h.Operations {
			if !isContractOperation(op) {
				return fmt.Errorf("compose: extension %q hooks operation %q, which the contract does not have", e.Name, op)
			}
		}
	}
	return nil
}

func isContractOperation(opID string) bool {
	for _, p := range agentPolicies {
		if p.Op == opID {
			return true
		}
	}
	return false
}
