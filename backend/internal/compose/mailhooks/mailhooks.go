// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailhooks serves the mail-capture hooks the registered units declare.
package mailhooks

// The mail-capture hooks of the registered units. This adapter owns the order:
// unit name, each hook seeing the previous result. An error, a panic or a slow
// call is a failure: the chain stops and core decides nothing for the mail. A
// change that names no declared reason is ignored. The checks that keep a
// colleague's decision whole are in capture/mailhooks.go.

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// mailHookLimit is how long one hook may run. The hook's context ends then. A
// hook that ignores it is abandoned and its answer dropped.
const mailHookLimit = 250 * time.Millisecond

const (
	mailHookParties = "parties"
	mailHookDeal    = "deal"
)

// units reads the served units. It is called at each hook, because capture is
// composed before the units are registered. The pool backs a hook's Lookup.
type units struct {
	served func() []extension.Extension
	pool   *pgxpool.Pool
}

// New is the capture hooks of the units that served lists.
func New(served func() []extension.Extension, pool *pgxpool.Pool) capture.MailHooks {
	u := units{served: served, pool: pool}
	return capture.MailHooks{Active: u.any, Parties: u.runParties, Verdict: u.runDeal}
}

// withHook are the units that registered a hook, in unit-name order.
func (u units) withHook(has func(extension.MailHooks) bool) []extension.Extension {
	var found []extension.Extension
	for _, unit := range u.served() {
		if has(unit.MailHooks) {
			found = append(found, unit)
		}
	}
	slices.SortFunc(found, func(a, b extension.Extension) int { return cmp.Compare(a.Name, b.Name) })
	return found
}

// callMailHook runs fn with the limit and turns a panic or a slow call into an
// error. fn gets a context that ends at the limit, detached from capture's own
// so a cancelled capture does not leave a half-read hook behind.
func callMailHook[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	type answer struct {
		value T
		err   error
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mailHookLimit)
	defer cancel()
	done := make(chan answer, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- answer{err: fmt.Errorf("the hook panicked: %v", r)}
			}
		}()
		value, err := fn(ctx)
		done <- answer{value, err}
	}()
	select {
	case a := <-done:
		return a.value, a.err
	case <-ctx.Done():
		var zero T
		return zero, fmt.Errorf("the hook ran longer than %s", mailHookLimit)
	}
}

// failedChange is what a failed hook leaves in the record. It decided nothing.
func failedChange(unit extension.Extension, hook string) capture.HookChange {
	return capture.HookChange{Unit: string(unit.Name), Hook: hook, Result: capture.HookFailed}
}

// change is one hook's change. A reason the unit did not declare makes it ignored.
func change(unit extension.Extension, hook, reason string) capture.HookChange {
	c := capture.HookChange{Unit: string(unit.Name), Hook: hook, Reason: reason, Result: capture.HookChanged}
	if !slices.Contains(unit.MailHooks.Reasons, reason) {
		c.Result = capture.HookIgnored
	}
	return c
}

func (u units) runParties(ctx context.Context, view capture.MailView, parties capture.MailParties) (capture.MailParties, []capture.HookChange) {
	var changes []capture.HookChange
	for _, unit := range u.withHook(func(h extension.MailHooks) bool { return h.Parties != nil }) {
		in := extension.MailParties{Contacts: uuidStrings(parties.Contacts), Companies: uuidStrings(parties.Companies)}
		out, err := callMailHook(ctx, func(c context.Context) (extension.MailParties, error) {
			return unit.MailHooks.Parties(c, u.extensionMailView(c, view), in)
		})
		if err != nil {
			slog.Warn("compose: a mail hook failed, so the mail is left undecided", "unit", unit.Name, "hook", mailHookParties, "err", err)
			return parties, append(changes, failedChange(unit, mailHookParties))
		}
		next, ok := partiesFromHook(out)
		if !ok || (slices.Equal(next.Contacts, parties.Contacts) && slices.Equal(next.Companies, parties.Companies)) {
			continue
		}
		changed := change(unit, mailHookParties, out.Reason)
		changes = append(changes, changed)
		if changed.Result == capture.HookChanged {
			parties = next
		}
	}
	return parties, changes
}

func (u units) runDeal(ctx context.Context, view capture.MailView, verdict capture.DealVerdict) (capture.DealVerdict, []capture.HookChange) {
	var changes []capture.HookChange
	for _, unit := range u.withHook(func(h extension.MailHooks) bool { return h.Deal != nil }) {
		in := dealForHook(verdict)
		out, err := callMailHook(ctx, func(c context.Context) (extension.MailDeal, error) {
			return unit.MailHooks.Deal(c, u.extensionMailView(c, view), in)
		})
		if err != nil {
			slog.Warn("compose: a mail hook failed, so the mail is left undecided", "unit", unit.Name, "hook", mailHookDeal, "err", err)
			return verdict, append(changes, failedChange(unit, mailHookDeal))
		}
		next, ok := dealFromHook(verdict, out)
		if !ok {
			continue
		}
		changed := change(unit, mailHookDeal, out.Reason)
		changes = append(changes, changed)
		if changed.Result == capture.HookChanged {
			verdict = next
		}
	}
	return verdict, changes
}

func (u units) extensionMailView(ctx context.Context, v capture.MailView) extension.MailView {
	return extension.MailView{
		Activity: v.Activity.String(), Meeting: v.Meeting, Subject: v.Subject, Body: v.Body,
		Addresses: slices.Clone(v.Addresses), Lookup: lookupFor(ctx, u.pool),
	}
}

func uuidStrings(list []ids.UUID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}
	return out
}

func uuidsFromStrings(list []string) ([]ids.UUID, bool) {
	out := make([]ids.UUID, len(list))
	for i, s := range list {
		id, err := ids.Parse(s)
		if err != nil {
			return nil, false
		}
		out[i] = id
	}
	return out, true
}

func partiesFromHook(p extension.MailParties) (capture.MailParties, bool) {
	contacts, ok := uuidsFromStrings(p.Contacts)
	if !ok {
		return capture.MailParties{}, false
	}
	companies, ok := uuidsFromStrings(p.Companies)
	return capture.MailParties{Contacts: contacts, Companies: companies}, ok
}

func dealForHook(v capture.DealVerdict) extension.MailDeal {
	out := extension.MailDeal{Outcome: string(v.Outcome), Candidates: uuidStrings(v.Candidates)}
	if !v.Deal.IsZero() {
		out.Deal = v.Deal.String()
	}
	return out
}

// dealFromHook turns a hook's answer into a verdict. It answers false when the
// answer is unchanged or cannot be read; capture then checks what it can.
func dealFromHook(prev capture.DealVerdict, out extension.MailDeal) (capture.DealVerdict, bool) {
	next := prev
	next.Outcome = capture.DealOutcome(out.Outcome)
	next.Deal = ids.UUID{}
	if out.Deal != "" {
		id, err := ids.Parse(out.Deal)
		if err != nil {
			return prev, false
		}
		next.Deal = id
	}
	candidates, ok := uuidsFromStrings(out.Candidates)
	if !ok {
		return prev, false
	}
	next.Candidates = candidates
	if next.Outcome == prev.Outcome && next.Deal == prev.Deal && slices.Equal(next.Candidates, prev.Candidates) {
		return prev, false
	}
	return next, true
}

// anyMailHook is false on an installation where no unit registered a hook.
func (u units) any() bool {
	return len(u.withHook(func(h extension.MailHooks) bool { return h.Parties != nil || h.Deal != nil })) > 0
}
