// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package subjectrightsrules is the core-internal registry behind a pack's
// request deadlines. The contract lives on the published surface
// backend/pkg/extension/subjectrights; this package keeps the registry and
// re-exports the types, the same split as messagingrules.
package subjectrightsrules

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	pub "github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// The published types, aliased so a set declared by an extension and one a
// core caller registers are the same type.
type (
	// Rules is one jurisdiction's request deadlines.
	Rules = pub.Rules
	// Deadline is one kind's deadline.
	Deadline = pub.Deadline
	// RequestKind is the right a request exercises.
	RequestKind = pub.RequestKind
)

var (
	mu    sync.RWMutex
	rules = map[jurisdiction.Code]Rules{}
)

// Register is called once per declared set at boot. An invalid or duplicate
// set is a wiring defect the compose preflight refuses first; the panic here
// is the backstop.
func Register(r Rules) {
	mu.Lock()
	defer mu.Unlock()
	if err := r.Validate(); err != nil {
		panic(fmt.Sprintf("subjectrightsrules: %v", err))
	}
	if _, dup := rules[r.Jurisdiction]; dup {
		panic(fmt.Sprintf("subjectrightsrules: rules for %q registered twice", r.Jurisdiction))
	}
	rules[r.Jurisdiction] = clone(r)
}

// For returns the rules a jurisdiction declares; ok is false when it declares
// none or the build does not carry it.
func For(code jurisdiction.Code) (Rules, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := rules[code]
	if !ok {
		return Rules{}, false
	}
	return clone(r), true
}

// clone keeps a caller from editing the registered set through the map or
// slice it was handed.
func clone(r Rules) Rules {
	out := r
	out.Instruments = slices.Clone(r.Instruments)
	out.Deadlines = maps.Clone(r.Deadlines)
	return out
}

// GDPRRule names the fallback deadline: one calendar month from receipt, GDPR
// Art. 12(3).
const GDPRRule = "gdpr"

// GDPR is that fallback deadline. A month, not thirty days, and clamped to the
// month's last day: one month after 31 January is 28 February (Deadline.Due).
// Held by TestOneCalendarMonthFollowsTheShortMonths.
var GDPR = Deadline{Period: jurisdiction.Period{Months: 1}}

// Resolve is the one lookup every request writer and the admin page make:
// the deadline a jurisdiction gives one kind, and the rule that names it. A
// jurisdiction that declares no deadline for the kind, and an empty code, get
// the GDPR month.
func Resolve(code jurisdiction.Code, kind RequestKind) (Deadline, string) {
	if code == "" {
		return GDPR, GDPRRule
	}
	rules, ok := For(code)
	if !ok {
		return GDPR, GDPRRule
	}
	deadline, ok := rules.Deadlines[kind]
	if !ok {
		return GDPR, GDPRRule
	}
	return deadline, string(code) + " v" + strconv.Itoa(rules.Version)
}
