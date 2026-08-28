// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// timeUnits are the durations this reader can resolve without compiling the
// unit. The manifest is derived from SOURCE, so `5 * time.Minute` has to be
// computed here rather than evaluated — and the set is closed deliberately:
// a unit that needs a skew this cannot spell is a unit asking for a bound
// nobody reading the declaration would guess.
var timeUnits = map[string]time.Duration{
	"Nanosecond":  time.Nanosecond,
	"Microsecond": time.Microsecond,
	"Millisecond": time.Millisecond,
	"Second":      time.Second,
	"Minute":      time.Minute,
	"Hour":        time.Hour,
}

// readInbound reads an Inbound field's slice literal into the manifest's
// declared endpoints.
//
// What it records is the installation's ANONYMOUS surface: every path a party
// with no session can POST to, with the bounds each one asked for. That is the
// list an operator needs before anything runs, and it is not derivable from
// anywhere else — a mounted route says nothing about which unit asked for it.
func (r *unitReader) readInbound(expr ast.Expr, file *ast.File) ([]inboundEndpoint, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Inbound must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]inboundEndpoint, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		endpoint, err := r.readInboundEndpoint(elt, ext)
		if err != nil {
			return nil, err
		}
		// Within one unit, because that is all this reader can see. The
		// cross-unit collision is boot's to refuse, where the whole composed
		// set is in hand.
		if seen[endpoint.Slug] {
			return nil, r.errAt(elt, "inbound endpoint %q declared twice", endpoint.Slug)
		}
		seen[endpoint.Slug] = true
		out = append(out, endpoint)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// readInboundEndpoint reads one extension.InboundEndpoint literal and validates
// it through the same published grammar the boot preflight runs, so a
// declaration this generator accepts is one the boot accepts too.
func (r *unitReader) readInboundEndpoint(elt ast.Expr, ext string) (inboundEndpoint, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "InboundEndpoint")) {
		return inboundEndpoint{}, r.errAt(elt, "an Inbound entry must be an extension.InboundEndpoint literal")
	}
	var endpoint inboundEndpoint
	// Handle is a func value and is deliberately NOT read: a manifest an
	// operator reads describes what is offered, and a function identifier tells
	// them nothing. Its presence is still required, which boot checks against
	// the live declaration — see registerInbound.
	var sawHandler, handleNil bool
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return inboundEndpoint{}, r.errAt(e, "InboundEndpoint fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return inboundEndpoint{}, r.errAt(kv.Key, "InboundEndpoint fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Slug":
			endpoint.Slug, err = r.stringLit(kv.Value, "InboundEndpoint.Slug")
		case "Secret":
			endpoint.Secret, err = r.stringLit(kv.Value, "InboundEndpoint.Secret")
		case "MaxBody":
			endpoint.MaxBody, err = r.intLit(kv.Value, "InboundEndpoint.MaxBody")
		case "Skew":
			endpoint.SkewSeconds, err = r.durationSeconds(kv.Value, ext, "InboundEndpoint.Skew")
		case "Rate":
			endpoint.Rate, err = r.readInboundRate(kv.Value, ext)
		case "Handle":
			// A nil spelling is caught HERE rather than left to be merely
			// "present": unlike a Tool's or a Job's, an InboundEndpoint has
			// no inert form — every declared edge mounts and serves, and
			// extension.InboundEndpoint.Validate() unconditionally refuses
			// e.Handle == nil at boot. Accepting `Handle: nil` here would let
			// generation succeed on a declaration boot can never start,
			// moving the failure from a `make composition` a unit author
			// runs locally to a shipped binary's boot log.
			if isStaticallyNilInboundHandle(kv.Value, ext) {
				handleNil = true
			} else {
				sawHandler = true
			}
		default:
			err = r.errAt(kv, "InboundEndpoint field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return inboundEndpoint{}, err
		}
	}
	if handleNil {
		return inboundEndpoint{}, r.errAt(lit, "inbound endpoint %q declares Handle: nil — an inbound endpoint has no inert form; every declared edge mounts and must serve a real handler", endpoint.Slug)
	}
	if !sawHandler {
		return inboundEndpoint{}, r.errAt(lit, "inbound endpoint %q declares no Handle", endpoint.Slug)
	}
	if err := declaredInboundEndpoint(endpoint).Validate(); err != nil {
		return inboundEndpoint{}, r.errPos(lit, "%v", err)
	}
	return endpoint, nil
}

// isStaticallyNilInboundHandle reports whether an InboundEndpoint.Handle
// expression is nil at the declaration.
//
// Two spellings, for the reason isStaticallyNil (Tool.Handle) gives: the bare
// `nil`, and a conversion through the published extension.InboundHandler type.
// The CallExpr arm checks the callee, not just the argument count, for the
// same reason stated there — a syntactic conversion and an ordinary
// one-argument call parse identically.
func isStaticallyNilInboundHandle(expr ast.Expr, ext string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "nil"
	case *ast.CallExpr:
		return len(e.Args) == 1 && isSelector(e.Fun, ext, "InboundHandler") && isStaticallyNilInboundHandle(e.Args[0], ext)
	case *ast.ParenExpr:
		return isStaticallyNilInboundHandle(e.X, ext)
	}
	return false
}

// readInboundRate reads the two metering buckets.
func (r *unitReader) readInboundRate(expr ast.Expr, ext string) (inboundRate, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "InboundRate")) {
		return inboundRate{}, r.errAt(expr, "InboundEndpoint.Rate must be an extension.InboundRate literal")
	}
	var rate inboundRate
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return inboundRate{}, r.errAt(e, "InboundRate fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return inboundRate{}, r.errAt(kv.Key, "InboundRate fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "PerIP":
			rate.PerIP, err = r.readRate(kv.Value, ext, "InboundRate.PerIP")
		case "PerEndpoint":
			rate.PerEndpoint, err = r.readRate(kv.Value, ext, "InboundRate.PerEndpoint")
		default:
			err = r.errAt(kv, "InboundRate field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return inboundRate{}, err
		}
	}
	return rate, nil
}

// readRate reads one allowance.
func (r *unitReader) readRate(expr ast.Expr, ext, field string) (rateRequest, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Rate")) {
		return rateRequest{}, r.errAt(expr, "%s must be an extension.Rate literal", field)
	}
	var out rateRequest
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return rateRequest{}, r.errAt(e, "%s fields must be keyed", field)
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return rateRequest{}, r.errAt(kv.Key, "%s fields must be keyed by name", field)
		}
		var err error
		switch k.Name {
		case "Limit":
			var limit int64
			limit, err = r.intLit(kv.Value, field+".Limit")
			out.Limit = int(limit)
		case "Window":
			out.WindowSeconds, err = r.durationSeconds(kv.Value, ext, field+".Window")
		default:
			err = r.errAt(kv, "%s field %s is not derivable by this generator", field, k.Name)
		}
		if err != nil {
			return rateRequest{}, err
		}
	}
	return out, nil
}

// intLit computes an integer written as a literal or as an arithmetic
// expression over literals — `64 << 10` is how a byte cap is spelled here and
// reads far better than 65536, so refusing it would push a unit author into the
// less readable form to satisfy a generator.
//
// The grammar is closed to shift, multiply and add over literals. Nothing here
// resolves an identifier: a cap that depends on a named constant is a cap the
// manifest could not state without compiling the unit.
func (r *unitReader) intLit(expr ast.Expr, field string) (int64, error) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.INT {
			return 0, r.errAt(expr, "%s must be an integer literal", field)
		}
		n, err := strconv.ParseInt(v.Value, 0, 64)
		if err != nil {
			return 0, r.errAt(expr, "%s is not an integer this generator can read: %v", field, err)
		}
		return n, nil
	case *ast.ParenExpr:
		return r.intLit(v.X, field)
	case *ast.BinaryExpr:
		left, err := r.intLit(v.X, field)
		if err != nil {
			return 0, err
		}
		right, err := r.intLit(v.Y, field)
		if err != nil {
			return 0, err
		}
		switch v.Op {
		case token.SHL:
			// Bounded before shifting: a shift count Go itself would accept can
			// still overflow int64 silently, and a byte cap that wrapped
			// negative would be read by Validate as "no cap declared".
			if right < 0 || right > 62 || left < 0 {
				return 0, r.errAt(expr, "%s shifts out of range", field)
			}
			return left << uint(right), nil
		case token.MUL:
			// Checked the same way SHL is, and for the same reason: this
			// generator's own int64 multiplication wraps silently at run
			// time, where the compiler's constant arithmetic is arbitrary
			// precision and would refuse a genuinely overflowing literal —
			// so an unchecked `*` here can derive a wrapped, wrong manifest
			// number instead of failing the way the source itself does.
			product := left * right
			if left != 0 && product/left != right {
				return 0, r.errAt(expr, "%s overflows the 64-bit range it must fit in", field)
			}
			return product, nil
		case token.ADD:
			// Checked the standard way: same-signed operands whose sum comes
			// back a different sign wrapped, for the reason MUL above gives.
			sum := left + right
			if (left > 0 && right > 0 && sum < 0) || (left < 0 && right < 0 && sum > 0) {
				return 0, r.errAt(expr, "%s overflows the 64-bit range it must fit in", field)
			}
			return sum, nil
		}
		return 0, r.errAt(expr, "%s uses %s, and this generator reads only <<, * and + over literals", field, v.Op)
	}
	return 0, r.errAt(expr, "%s must be an integer literal or an arithmetic expression over literals", field)
}

// durationSeconds computes a duration written as `N * time.Unit` or as a bare
// `time.Unit`, and answers it in WHOLE SECONDS.
//
// Seconds because that is what the manifest publishes and what an operator
// compares against a provider's own retry window; a nanosecond count in a
// document a human reads is a number nobody checks. A sub-second value is
// refused rather than rounded — rounding a skew of 500ms to zero would publish
// "no freshness bound" for a declaration that asked for one.
func (r *unitReader) durationSeconds(expr ast.Expr, ext, field string) (int64, error) {
	total, err := r.duration(expr, ext, field)
	if err != nil {
		return 0, err
	}
	if total%time.Second != 0 {
		return 0, r.errAt(expr, "%s is %s — this surface is published in whole seconds", field, total)
	}
	return int64(total / time.Second), nil
}

func (r *unitReader) duration(expr ast.Expr, ext, field string) (time.Duration, error) {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return r.duration(v.X, ext, field)
	case *ast.SelectorExpr:
		return r.timeUnit(v, ext, field)
	case *ast.BinaryExpr:
		if v.Op != token.MUL {
			return 0, r.errAt(expr, "%s must be written as N * time.Unit", field)
		}
		// Either order — `5 * time.Minute` and `time.Minute * 5` are the same
		// declaration and a generator that read one and refused the other would
		// be a style rule wearing a grammar's clothes.
		if unit, err := r.timeUnitOrNil(v.Y, ext); err == nil && unit != 0 {
			n, err := r.intLit(v.X, field)
			if err != nil {
				return 0, err
			}
			return time.Duration(n) * unit, nil
		}
		if unit, err := r.timeUnitOrNil(v.X, ext); err == nil && unit != 0 {
			n, err := r.intLit(v.Y, field)
			if err != nil {
				return 0, err
			}
			return time.Duration(n) * unit, nil
		}
		// Both operands failed to be a unit. If one of them at least NAMES the
		// time package, say which unit was not recognised — "must multiply a
		// literal by a time unit" sends an author looking at the wrong operand.
		for _, operand := range []ast.Expr{v.X, v.Y} {
			if sel, ok := operand.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "time" {
					return 0, r.errAt(operand, "%s names time.%s, and this generator reads only the time package's units of duration", field, sel.Sel.Name)
				}
			}
		}
		return 0, r.errAt(expr, "%s must multiply a literal by a time unit", field)
	}
	return 0, r.errAt(expr, "%s must be a duration written as N * time.Unit", field)
}

func (r *unitReader) timeUnit(sel *ast.SelectorExpr, ext, field string) (time.Duration, error) {
	unit, err := r.timeUnitOrNil(sel, ext)
	if err != nil || unit == 0 {
		return 0, r.errAt(sel, "%s names %s, and this generator reads only the time package's units", field, sel.Sel.Name)
	}
	return unit, nil
}

// timeUnitOrNil answers the unit a selector names, or zero when the selector is
// not a time unit at all. It reports an error only for a malformed selector,
// so the caller can try the other operand of a multiplication.
func (r *unitReader) timeUnitOrNil(expr ast.Expr, ext string) (time.Duration, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return 0, nil
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return 0, nil
	}
	// The time package, never the extension package: a published extension
	// constant that happened to be named Minute would otherwise resolve here.
	if pkg.Name != "time" || pkg.Name == ext {
		return 0, nil
	}
	return timeUnits[sel.Sel.Name], nil
}

// declaredInboundEndpoint rebuilds the published type from what was read, so
// the generator runs the unit's own grammar rather than a second copy of it.
//
// Handle is set to a non-nil stub because Validate requires one and this reader
// deliberately does not read the function value — the presence check above is
// what stands in for it.
func declaredInboundEndpoint(e inboundEndpoint) extension.InboundEndpoint {
	return extension.InboundEndpoint{
		Slug:    e.Slug,
		Secret:  e.Secret,
		MaxBody: e.MaxBody,
		Skew:    time.Duration(e.SkewSeconds) * time.Second,
		Rate: extension.InboundRate{
			PerIP:       extension.Rate{Limit: e.Rate.PerIP.Limit, Window: time.Duration(e.Rate.PerIP.WindowSeconds) * time.Second},
			PerEndpoint: extension.Rate{Limit: e.Rate.PerEndpoint.Limit, Window: time.Duration(e.Rate.PerEndpoint.WindowSeconds) * time.Second},
		},
		Handle: func(context.Context, extension.Runtime, extension.InboundRequest) (extension.InboundOutcome, error) {
			return extension.InboundAccepted, nil
		},
	}
}
