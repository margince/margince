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

// operationHooks runs every composed unit's hooks on a contract operation, in
// unit order. It sits outside the agent gate, so the gate judges the body core
// will run. The operation is named by agentPolicies, the generated route table
// the gate reads too.
func operationHooks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opID := agentPolicies[r.Method+" "+chi.RouteContext(r.Context()).RoutePattern()].Op
		units := unitsHooking(opID)
		if opID == "" || len(units) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, httperr.MaxBodyBytes+1))
		if err != nil || len(body) > httperr.MaxBodyBytes {
			httperr.Write(w, r, fmt.Errorf("%w: the request body is unreadable or over %d bytes", apperrors.ErrInvalidArgument, httperr.MaxBodyBytes))
			return
		}
		op := &extension.Operation{ID: opID, Body: body}
		for _, e := range units {
			if err := runOperationHooks(r, e, opID, op); err != nil {
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

// runOperationHooks filters on opID, the route's own, so a hook that rewrites
// op.ID cannot steer which hooks run after it.
func runOperationHooks(r *http.Request, e extension.Extension, opID string, op *extension.Operation) error {
	rt := runtimeFor(r.Context(), string(e.Name), string(e.Version), "hook/operation/"+opID, boundExtensionRuntime())
	defer rt.release()
	for _, hook := range e.OperationHooks {
		if !slices.Contains(hook.Operations, opID) {
			continue
		}
		if err := hook.Handle(r.Context(), rt, op); err != nil {
			return err
		}
	}
	return nil
}

// preflightHooks refuses a hook with no handler, entity, action or operation,
// and one on a record kind or operation agents cannot reach. Sign-in,
// credentials, consent and the other human-only doors stay out of a unit's
// reach, and so do its own ext_* rows, which run no hooks.
func preflightHooks(e extension.Extension) error {
	records := hookableRecords()
	for _, h := range e.WriteHooks {
		if h.Handle == nil || h.Entity == "" || len(h.Actions) == 0 {
			return fmt.Errorf("compose: extension %q declares a write hook on %q with no handler, entity or action", e.Name, h.Entity)
		}
		if !records[h.Entity] {
			return fmt.Errorf("compose: extension %q hooks writes to %q, which is not a record kind agents can reach", e.Name, h.Entity)
		}
	}
	for _, h := range e.OperationHooks {
		if h.Handle == nil || len(h.Operations) == 0 {
			return fmt.Errorf("compose: extension %q declares an operation hook with no handler or operation", e.Name)
		}
		for _, op := range h.Operations {
			if !hookableOperation(op) {
				return fmt.Errorf("compose: extension %q hooks operation %q, which is not a record operation agents can reach", e.Name, op)
			}
		}
	}
	return nil
}

// identityRecords are agent-reachable record kinds that are seats and their
// rights, which no unit may veto or read through a hook.
var identityRecords = []agentRecordType{recordTypeAppUser, recordTypeUserAdmin}

func hookable(p agentPolicy) bool {
	return p.Access == accessTool && p.RecordType != "" && !slices.Contains(identityRecords, p.RecordType)
}

func hookableOperation(opID string) bool {
	for _, p := range agentPolicies {
		if p.Op == opID {
			return hookable(p)
		}
	}
	return false
}

func hookableRecords() map[string]bool {
	records := map[string]bool{}
	for _, p := range agentPolicies {
		if hookable(p) {
			records[string(p.RecordType)] = true
		}
	}
	return records
}
