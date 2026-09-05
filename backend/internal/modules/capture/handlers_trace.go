// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The /capture/activity transport. compose embeds TraceHandlers so these
// methods shadow the generated 501 stubs; a role that composes no trace store
// leaves the field zero and both operations answer an honest 503 rather than
// nil-dereferencing.
//
// The gate is in the STORE, not here: ListWorkspace takes it, ListMine has none
// to take. This layer decodes, maps and writes.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TraceHandlers is the trace read transport.
type TraceHandlers struct {
	store *TraceStore
	// payloadCapture reports the deployment's capture.trace_payloads posture on
	// every answer, so a client can tell "the operator did not enable this" from
	// "this row has no payload". It is read from the composed config rather than
	// inferred from the rows: a window in which every message happened to have no
	// subject would otherwise look like the posture was off.
	payloadCapture bool
	// senderPass answers when the sender verdict next runs. Injected because the
	// schedule lives in the job queue, which this module does not own and must
	// not learn to read: compose holds the pool and the seam.
	//
	// Nil is a deployment that composed no queue reader, and it answers the
	// screen with no clock at all rather than a zero one — "next pass at
	// 01:00 on 1 January 1970" is worse than saying nothing.
	senderPass VerdictPass
}

// VerdictPass answers when one verdict pass next runs. Once per read, not per
// row: the schedule is a property of the installation.
type VerdictPass func(ctx context.Context) (VerdictClock, error)

// VerdictClock is one pass's schedule as a screen can state it.
type VerdictClock struct {
	// Every is the declared cadence. Zero where no clock runs this pass, which
	// is a different sentence from a next time this deployment cannot compute.
	Every time.Duration
	// Running says a pass is in flight right now.
	Running bool
	// NextAt is when the next pass runs, nil when unknowable.
	NextAt *time.Time
}

// NewTraceHandlers wires the transport over a trace store.
func NewTraceHandlers(store *TraceStore, payloadCapture bool, senderPass VerdictPass) TraceHandlers {
	return TraceHandlers{store: store, payloadCapture: payloadCapture, senderPass: senderPass}
}

// ListMyCaptureActivity answers for the caller's own connections.
func (h TraceHandlers) ListMyCaptureActivity(w http.ResponseWriter, r *http.Request,
	params crmcontracts.ListMyCaptureActivityParams,
) {
	h.answer(w, r, func() (TraceWindow, error) {
		return h.store.ListMine(r.Context(), params.Cursor, params.Limit)
	})
}

// ListWorkspaceCaptureActivity answers for the workspace's shared channels.
func (h TraceHandlers) ListWorkspaceCaptureActivity(w http.ResponseWriter, r *http.Request,
	params crmcontracts.ListWorkspaceCaptureActivityParams,
) {
	h.answer(w, r, func() (TraceWindow, error) {
		return h.store.ListWorkspace(r.Context(), params.Cursor, params.Limit)
	})
}

// answer runs one read and writes it, so the two operations differ only in
// which read they name.
func (h TraceHandlers) answer(w http.ResponseWriter, r *http.Request, read func() (TraceWindow, error)) {
	if h.store == nil {
		httperr.ServiceUnavailable(w, r,
			"this deployment composed no capture pipeline, so there is no capture activity to read")
		return
	}
	window, err := read()
	if err != nil {
		WriteTraceErr(w, r, err)
		return
	}
	response := traceResponse(window, h.payloadCapture)
	if h.senderPass != nil {
		pass, err := h.senderPass(r.Context())
		if err != nil {
			// The window is the answer; the clock is what makes waiting
			// legible. A queue this read could not ask still leaves a screen
			// that works, so the counters go out without it rather than
			// failing a read nobody asked a schedule of.
			slog.WarnContext(r.Context(), "capture: reading the sender verdict schedule", "error", err)
		} else {
			response.SenderVerdict = VerdictClockResponse(pass)
		}
	}
	httperr.WriteJSON(w, http.StatusOK, response)
}

// WriteTraceErr maps this module's own refusal onto the wire. Everything else
// is already an apperrors sentinel and travels unchanged.
//
// Exported so the pipeline-trace doors in compose answer identically: they read
// the same store and can return the same sentinel, and without one mapping the
// window read 503s with a sentence naming what is missing while the drawer
// beside it 500s "internal" — the same condition, two answers, one of them
// useless to whoever is reading the log.
func WriteTraceErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errNoCallingMember) {
		// Not 403: the caller is not being denied their own traffic — there is
		// simply no member on this invocation to have any.
		httperr.ServiceUnavailable(w, r,
			"this read answers for the calling member, and this invocation has none")
		return
	}
	httperr.Write(w, r, err)
}

// traceResponse maps the store's answer onto the contract's shape.
func traceResponse(window TraceWindow, payloadCapture bool) crmcontracts.CaptureActivityResponse {
	entries := make([]crmcontracts.CaptureTraceEntry, 0, len(window.Entries))
	for _, row := range window.Entries {
		entries = append(entries, traceEntryResponse(row))
	}
	return crmcontracts.CaptureActivityResponse{
		Funnel:                traceFunnelResponse(window.Funnel),
		Data:                  entries,
		Page:                  crmcontracts.PageInfo{NextCursor: nullableString(window.Next)},
		PayloadCaptureEnabled: payloadCapture,
		WindowHours:           TraceWindowHours,
	}
}

// VerdictClockResponse renders one pass's schedule. Exported because the held
// threads answer carries the OTHER pass in the same shape, and two renderings
// of one contract type is how the two screens come to phrase the same fact
// differently.
func VerdictClockResponse(clock VerdictClock) *crmcontracts.CaptureVerdictClock {
	return &crmcontracts.CaptureVerdictClock{
		EverySeconds: int(clock.Every / time.Second),
		Running:      clock.Running,
		NextPassAt:   clock.NextAt,
	}
}

func traceFunnelResponse(funnel map[string]int) crmcontracts.CaptureActivityFunnel {
	out := crmcontracts.CaptureActivityFunnel{}
	for outcome, n := range funnel {
		count := n
		switch TraceOutcome(outcome) {
		case TraceCaptured:
			out.Captured = &count
		case TraceInternal:
			out.Internal = &count
		case TraceSuppressed:
			out.Suppressed = &count
		case TraceDeferred:
			out.Deferred = &count
		case TraceFault:
			out.Fault = &count
		}
	}
	return out
}

func traceEntryResponse(row TraceRow) crmcontracts.CaptureTraceEntry {
	entry := crmcontracts.CaptureTraceEntry{
		Id:           openapi_types.UUID(row.ID),
		Connector:    row.Connector,
		Outcome:      crmcontracts.CaptureTraceEntryOutcome(row.Outcome),
		OutcomeNow:   crmcontracts.CaptureTraceEntryOutcomeNow(row.OutcomeNow),
		Reason:       nullableString(row.Reason),
		ActivityId:   traceUUID(row.ActivityID),
		Counterparty: nullableString(row.Counterparty),
		Subject:      nullableString(row.Subject),
		OccurredAt:   row.OccurredAt,
	}
	if row.Resolution != nil {
		entry.Resolution = &crmcontracts.CaptureTraceResolution{
			Status:     crmcontracts.CaptureTraceResolutionStatus(row.Resolution.Status),
			Kind:       nullableString(row.Resolution.Kind),
			ResolvedAt: row.Resolution.ResolvedAt,
		}
	}
	return entry
}

// traceUUID renders an optional id for the wire.
func traceUUID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	out := openapi_types.UUID(*id)
	return &out
}

// nullableString renders an absent value as a JSON null rather than an empty
// string: "" and "there is none" are different answers, and a client rendering
// a reason must be able to tell them apart.
func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
