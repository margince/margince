// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The chassis's observability surface: correlation-aware logging, the
// access log and the readiness probe. Everything here is transport plumbing —
// what to check is injected by the composition layer. The /metrics endpoint is
// next door in metrics.go.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LogHandler builds the slog backend from the operator's --log-level and
// --log-format values. It lives here so every process role shares one
// level/format vocabulary and one "a typo is a boot error" rule.
func LogHandler(w io.Writer, level, format string) (slog.Handler, error) {
	var lv slog.LevelVar
	switch level {
	case "debug":
		lv.Set(slog.LevelDebug)
	case "info":
		lv.Set(slog.LevelInfo)
	case "warn":
		lv.Set(slog.LevelWarn)
	case "error":
		lv.Set(slog.LevelError)
	default:
		return nil, fmt.Errorf("--log-level %q: want debug, info, warn, or error", level)
	}
	opts := &slog.HandlerOptions{Level: &lv}
	switch format {
	case "text":
		return slog.NewTextHandler(w, opts), nil
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("--log-format %q: want text or json", format)
	}
}

// InstallProcessLogger builds this role's logger from the operator's
// --log-level and --log-format AND makes it the process default, answering it
// for everything that takes a logger explicitly.
//
// THE SetDefault IS THE POINT, and it is what a role that only built a logger
// was missing. Plenty of code in this tree logs through the PACKAGE-LEVEL
// slog functions — slog.ErrorContext, slog.WarnContext — which reach
// slog.Default() and nothing else. jobs.faultFor is the case that matters
// most: a postponed tick records no attempt error anywhere, so its log line
// and the unit's own row are the entire trail an outage leaves in the process.
// Until a role installed its handler here, that line went to the stdlib
// default — text, on stderr — while every explicitly-logged line went to the
// operator's configured sink and format. A collector parsing the worker's JSON
// got an unstructured line for exactly the events it most wants, and nothing
// anywhere said so.
//
// It is also what makes the CORRELATION handler reach those call sites. The
// wrapper below enriches a record only when it is the handler doing the
// logging, so a package-level call against a bare default carried no
// correlation_id whichever context it was given — which is why fault.go used
// to attach the id by hand, and why it no longer has to.
//
// A role builds its logger ONCE, at boot, before anything serves. Nothing here
// guards against a second call: the process default is a single value by
// construction, and a role that installed two logs through whichever won.
func InstallProcessLogger(w io.Writer, level, format string) (*slog.Logger, error) {
	handler, err := LogHandler(w, level, format)
	if err != nil {
		return nil, err
	}
	logger := slog.New(WithCorrelation(handler))
	slog.SetDefault(logger)
	return logger, nil
}

// WithCorrelation wraps a slog.Handler so every record logged through a
// *Context method carries the request's correlation_id — the same id the
// Correlate middleware minted and every emitted event's trace links, so
// one grep joins log lines, audit rows, and bus events.
func WithCorrelation(h slog.Handler) slog.Handler {
	return &correlationHandler{inner: h}
}

type correlationHandler struct{ inner slog.Handler }

func (h *correlationHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *correlationHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id, ok := principal.CorrelationID(ctx); ok {
		rec.AddAttrs(slog.String("correlation_id", id.String()))
	}
	return h.inner.Handle(ctx, rec)
}

func (h *correlationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &correlationHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *correlationHandler) WithGroup(name string) slog.Handler {
	return &correlationHandler{inner: h.inner.WithGroup(name)}
}

// AccessLog logs one line per request (method, path, status, duration);
// the correlation_id rides in via the ctx-aware handler, so it must be
// mounted inside Correlate. The path is the request path, not the route
// pattern — the access log answers "what did clients ask", the metrics
// answer "how did routes behave".
//
// A path segment that is a bearer credential is redacted before the line is
// written, by shared/kernel/capabilitypath, which owns both the redaction and
// the list of routes that carry one. That list does NOT arrive as an argument
// here: it used to, and five of this function's six mount sites passed
// nothing, so a mount reached the log unredacted by saying less rather than
// by saying something wrong.
func AccessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", capabilitypath.Redact(r.URL.Path),
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

// statusRecorder captures the response status for the access log.
//
// wrote distinguishes "the handler answered 200" from "nobody answered": the
// zero value of status has to be the 200 net/http sends for a handler that just
// returns, so the field alone cannot tell those apart. The HTTP metrics need
// that distinction to record a panicking handler honestly -- see Measure.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

// WriteHeader records the FIRST status only, because that is the only one the
// client receives: net/http sends one status line and logs a "superfluous
// WriteHeader" for every call after it. Recording the last attempt instead --
// which this did -- reports a status that was never sent. A handler that
// answered 201 and then tried 500 on a later error path was logged, and
// counted, as a 500 the client never saw.
//
// The call is still forwarded, so net/http's own warning is not suppressed:
// a double WriteHeader is a handler bug, and hiding it here would remove the
// one signal that says so.
func (r *statusRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap exposes the wrapped writer so http.NewResponseController can reach
// the real connection: SSE streams and long-running tool calls need
// SetWriteDeadline and Flush, and an embedded-only wrapper silently swallows
// both.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// ReadyCheck is one named dependency probe for /readyz.
type ReadyCheck struct {
	Name  string
	Check func(context.Context) error
}

// Readyz answers the readiness probe: every injected dependency check
// must pass within a short deadline. Distinct from /healthz, which stays
// a dumb liveness answer — a wedged database must fail readiness (stop
// routing traffic here) without failing liveness (don't restart-loop the
// process the database outage didn't break).
//
// aiState rides the 200 body as a visibility line — "configured",
// "unconfigured", or "fake" — never a gate: an AI-unconfigured
// deployment is a legitimate, ready deployment (ai-operational-spec
// §2), so it is reported alongside "ready", not checked as a
// ReadyCheck that could turn it into a 503.
//
// embedState is the same shape of visibility line for the search
// module's embed store binding (Task 17): "active", "needs_reindex", or
// "reembedding". It is called once, after every check has passed, with
// the same deadline-bound ctx the checks ran under; a nil embedState (a
// role that wires no embed lane) and one whose own marker-read failed
// and already turned that into "unknown" both render identically —
// Readyz never inspects why, only what it's handed. Like the AI line,
// this NEVER gates: the embed store still serves N+1 reads correctly
// under a stale binding, so a drifted or unreadable marker is
// informational, not a reason to stop routing traffic here.
func Readyz(aiState string, embedState func(context.Context) string, checks ...ReadyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		// The probe body goes through the same writer the exposition does, for
		// the same reason: the first refused write stops the rest. There is
		// nothing to LOG here — a probe whose reader hung up has no channel
		// left to report on, and the orchestrator's own timeout is what says
		// so — but a half-written answer is still worth not assembling.
		body := &exposition{w: w}
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				// The dependency name is enough for the orchestrator; the
				// error text is for the server log, not the probe body.
				slog.ErrorContext(r.Context(), "readiness check failed", "dependency", c.Name, "err", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				body.printf("unready: %s\n", c.Name)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		body.printf("ready\n")
		// Each visibility line is written only by a role that wires the thing
		// it reports. The worker wires neither, and an empty "ai: " line is
		// noise an operator has to learn to ignore — worse than silence,
		// because it reads as a role whose AI state could not be determined.
		if aiState != "" {
			body.printf("ai: %s\n", aiState)
		}
		// Nothing is READ for a probe whose reader is gone, the same rule the
		// exposition takes: embedState resolves a marker, and resolving one
		// for a body that cannot be delivered is work with no reader.
		if body.gone() {
			return
		}
		// The embed line is written unconditionally, unlike the AI one: a nil
		// embedState and a marker-read that already failed into "unknown" are
		// deliberately indistinguishable here, so omitting it would turn one
		// of those two into an absence and the other into a value.
		embed := "unknown"
		if embedState != nil {
			embed = embedState(ctx)
		}
		body.printf("embed: %s\n", embed)
	}
}
