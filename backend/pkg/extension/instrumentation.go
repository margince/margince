// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
)

// Instrumentation lets a unit observe the whole process. It sees the root log
// handler, the HTTP chain, each job and subscription delivery, and recovered
// panics. In the browser it sees errors and navigation. It names no vendor.
//
// The core guards every call. A Start that fails or panics is logged, and the
// process boots without that unit's hooks. A hook that panics is logged, and
// the work it wrapped goes on.
type Instrumentation struct {
	// Start runs once per process (api, worker), before it serves or runs
	// work. Nil means the unit has no backend part.
	Start func(ctx context.Context, p Process) (Instruments, error)
	// Browser is a named export of the unit's frontend main module: a function
	// the SPA calls before its first render. Empty means no browser part.
	Browser string
	// Edge is served without a session at /v1/public/instrument/<unit>/, with
	// that prefix stripped. It takes GET and POST only, a body of at most
	// 1 MiB, and is held to a per-IP limit. The unit sets its own caching
	// headers. Nil means none.
	Edge http.Handler
}

// Process is what the core tells Start about the process it runs in.
type Process struct {
	Role    string       // "api" or "worker"
	Version string       // the build's release version; "dev" when unknown
	Logger  *slog.Logger // the base logger, not yet wrapped by any unit
	// RedactPath blanks the credential segment of a capability URL. A unit must
	// name anything derived from a request path through it.
	RedactPath func(path string) string
}

// Instruments are the hooks Start returns. Every one is optional.
//
// With several units, the first in the composed registry is outermost in every
// chain. If Start returns an error, its hooks are discarded. A Start that
// hangs blocks boot.
type Instruments struct {
	// LogHandler wraps the process's root log handler, once, as the logger is
	// built. Every record from every package passes through it, inside the
	// correlation wrapper, so it sees the correlation id.
	LogHandler func(slog.Handler) slog.Handler
	// HTTP wraps the whole request router, once, in the api. It sits inside
	// the panic catch and outside everything else. So no correlation id or
	// principal is on the context yet. r.RemoteAddr is still the proxy, before
	// the client address is resolved. The body is not yet capped.
	HTTP func(http.Handler) http.Handler
	// Work wraps every job and every extension subscription delivery. It must
	// call run on the calling goroutine. If the work panics, run re-raises that
	// panic there, and the work's panic passes through the hook unchanged. The
	// work runs once even if the hook never calls run, or panics before it.
	Work func(ctx context.Context, w Work, run func(context.Context) error) error
	// Panic is told of a panic the core has caught, at four places.
	// RecoverPanics reports an api request and the worker's event listener.
	// jobs.Govern reports any River job that panics out of its work. The
	// extension job tick and an extension subscription delivery report too.
	// A panic that other code recovers by itself reaches a unit only through
	// its LogHandler, at Error level. Every site also logs its panic at Error
	// with a "panic" attribute. A record with that attribute describes a
	// panic already handed to Panic, so a unit must not report it twice.
	Panic func(ctx context.Context, value any, stack []byte)
	// Shutdown runs when the process exits, so a unit can send what it still
	// holds. It gets a window of 2 seconds.
	Shutdown func(ctx context.Context) error
}

// Work names one unit of background work the core is about to run.
type Work struct {
	Kind string // WorkJob or WorkSubscription
	Name string // the River kind, or the subscription's name
	// Unit is the owning unit. It is set only for an extension subscription
	// delivery. jobs.Govern never sets it, so it is empty for every job, and an
	// extension job's Name is its River kind.
	Unit string
}

// The two kinds of Work. Every River job passes through jobs.Govern, so a job
// is not a separate "run" kind.
const (
	WorkJob          = "job"
	WorkSubscription = "subscription"
)

// browserExportPattern is a plain JavaScript identifier, at most 64 characters.
var browserExportPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

// Validate refuses a declaration that instruments nothing, and a Browser that
// is not a plain named export.
func (i Instrumentation) Validate() error {
	if i.Start == nil && i.Browser == "" && i.Edge == nil {
		return fmt.Errorf("%w: instrumentation declares no Start, Browser or Edge", ErrInvalid)
	}
	if i.Browser != "" && (!browserExportPattern.MatchString(i.Browser) || i.Browser == "default") {
		return fmt.Errorf("%w: instrumentation Browser %q is not a named export", ErrInvalid, i.Browser)
	}
	return nil
}
