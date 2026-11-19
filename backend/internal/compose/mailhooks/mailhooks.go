// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailhooks

// The mail-capture hooks of the registered units. This adapter owns the order:
// unit name, each hook seeing the previous result. An error, a panic or a slow
// call leaves the previous result as it was. The checks that keep a colleague's
// decision whole are in capture/mailhooks.go.

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// mailHookLimit is how long one hook may run. A hook has no context to cancel,
// so a call past the limit is abandoned and its answer dropped.
const mailHookLimit = 250 * time.Millisecond

const (
	mailHookParties = "parties"
	mailHookDeal    = "deal"
)

// units reads the served units. New calls it at each hook, because capture is
// composed before the units are registered.
type units func() []extension.Extension

// New is the capture hooks of the units that units serves.
func New(served func() []extension.Extension) capture.MailHooks {
	u := units(served)
	return capture.MailHooks{Active: u.any, Parties: u.runParties, Verdict: u.runDeal}
}

// withHook are the units that registered a hook, in unit-name order.
func (u units) withHook(has func(extension.MailHooks) bool) []extension.Extension {
	var found []extension.Extension
	for _, unit := range u() {
		if has(unit.MailHooks) {
			found = append(found, unit)
		}
	}
	slices.SortFunc(found, func(a, b extension.Extension) int { return cmp.Compare(a.Name, b.Name) })
	return found
}

// callMailHook runs fn with the limit and turns a panic or a slow call into an error.
func callMailHook[T any](fn func() (T, error)) (T, error) {
	type answer struct {
		value T
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- answer{err: fmt.Errorf("the hook panicked: %v", r)}
			}
		}()
		value, err := fn()
		done <- answer{value, err}
	}()
	timer := time.NewTimer(mailHookLimit)
	defer timer.Stop()
	select {
	case a := <-done:
		return a.value, a.err
	case <-timer.C:
		var zero T
		return zero, fmt.Errorf("the hook ran longer than %s", mailHookLimit)
	}
}

func (u units) runParties(view capture.MailView, parties capture.MailParties) (capture.MailParties, []capture.HookChange) {
	var changes []capture.HookChange
	for _, unit := range u.withHook(func(h extension.MailHooks) bool { return h.Parties != nil }) {
		in := extension.MailParties{Contacts: uuidStrings(parties.Contacts), Companies: uuidStrings(parties.Companies)}
		out, err := callMailHook(func() (extension.MailParties, error) { return unit.MailHooks.Parties(extensionMailView(view), in) })
		if err != nil {
			slog.Warn("compose: a mail hook failed and its result was ignored", "unit", unit.Name, "hook", mailHookParties, "err", err)
			continue
		}
		next, ok := partiesFromHook(out)
		if !ok || (slices.Equal(next.Contacts, parties.Contacts) && slices.Equal(next.Companies, parties.Companies)) {
			continue
		}
		parties = next
		changes = append(changes, capture.HookChange{Unit: string(unit.Name), Hook: mailHookParties, Reason: out.Reason})
	}
	return parties, changes
}

func (u units) runDeal(view capture.MailView, verdict capture.DealVerdict) (capture.DealVerdict, []capture.HookChange) {
	var changes []capture.HookChange
	for _, unit := range u.withHook(func(h extension.MailHooks) bool { return h.Deal != nil }) {
		in := dealForHook(verdict)
		out, err := callMailHook(func() (extension.MailDeal, error) { return unit.MailHooks.Deal(extensionMailView(view), in) })
		if err != nil {
			slog.Warn("compose: a mail hook failed and its result was ignored", "unit", unit.Name, "hook", mailHookDeal, "err", err)
			continue
		}
		next, ok := dealFromHook(verdict, out)
		if !ok {
			continue
		}
		verdict = next
		changes = append(changes, capture.HookChange{Unit: string(unit.Name), Hook: mailHookDeal, Reason: out.Reason})
	}
	return verdict, changes
}

func extensionMailView(v capture.MailView) extension.MailView {
	return extension.MailView{Activity: v.Activity.String(), Meeting: v.Meeting, Subject: v.Subject, Addresses: slices.Clone(v.Addresses)}
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
