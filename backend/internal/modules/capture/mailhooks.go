// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The two places an extension may adjust the deal rule. Compose has already
// run every unit's hook in order, with its error and time limit. This file
// holds what core insists on afterwards. The result must be one core could have
// reached, and it must not undo what a person decided. A result that fails is
// dropped and the previous one stays.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealBasisExtension is the verdict of an extension that linked a deal. No topic
// check follows it, because the extension decided on its own rules.
const DealBasisExtension DealBasis = "extension"

// HookChange is one extension's change to a result, and its reason.
type HookChange struct {
	Unit   string
	Hook   string
	Reason string
}

// MailView is what a hook may read about one mail.
type MailView struct {
	Activity  ids.ActivityID
	Meeting   bool
	Subject   string
	Addresses []string
}

// MailParties is the contacts and companies derived for a mail.
type MailParties struct {
	Contacts  []ids.UUID
	Companies []ids.UUID
}

// MailHooks are the two hook points. A nil function is no hook, and a hook that
// reports itself not Active costs the rule nothing, not even a read.
type MailHooks struct {
	Active  func() bool
	Parties func(MailView, MailParties) (MailParties, []HookChange)
	Verdict func(MailView, DealVerdict) (DealVerdict, []HookChange)
}

// mailView reads what a hook may see. It answers false when the deciding seat
// may not read the mail's content, so a held mail's subject never reaches a hook.
func mailView(ctx context.Context, tx pgx.Tx, facts dealFacts) (MailView, bool, error) {
	args := []any{facts.activity}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return MailView{}, false, err
	}
	if scope == "" {
		scope = sqlUnscoped
	}
	view := MailView{Activity: facts.activity, Meeting: facts.meeting}
	err = tx.QueryRow(ctx, `
		SELECT coalesce(a.subject, ''),
		       coalesce((SELECT array_agg(DISTINCT lower(p.address) ORDER BY lower(p.address))
		                   FROM activity_participant p WHERE p.activity_id = a.id AND p.address IS NOT NULL), '{}')
		  FROM activity a WHERE a.id = $1 AND (`+scope+`)`, args...).Scan(&view.Subject, &view.Addresses)
	if err == pgx.ErrNoRows {
		return MailView{}, false, nil
	}
	if err != nil {
		return MailView{}, false, fmt.Errorf("capture: reading the mail for the hooks: %w", err)
	}
	return view, true, nil
}

// adjustParties runs the parties hook over what core derived. It runs only
// where core derived something, so a seat that may read neither contacts nor
// companies gives a hook nothing to change.
func (h MailHooks) adjustParties(ctx context.Context, tx pgx.Tx, facts *dealFacts) error {
	if h.Parties == nil || !h.active() || !auth.ReadGranted(ctx, "contact") || !auth.ReadGranted(ctx, "company") {
		return nil
	}
	view, ok, err := mailView(ctx, tx, *facts)
	if err != nil || !ok {
		return err
	}
	out, changes := h.Parties(view, MailParties{Contacts: facts.contacts, Companies: facts.companies})
	if len(changes) == 0 || (slices.Equal(out.Contacts, facts.contacts) && slices.Equal(out.Companies, facts.companies)) {
		return nil
	}
	valid, err := partiesAreLive(ctx, tx, out)
	if err != nil || !valid {
		return err
	}
	facts.contacts, facts.companies = out.Contacts, out.Companies
	facts.hooks = append(facts.hooks, changes...)
	return nil
}

// partiesAreLive holds a hook to the shape core derives. Contacts are unique
// and live. Companies are unique, live, at most two, and not the installation's own.
func partiesAreLive(ctx context.Context, tx pgx.Tx, p MailParties) (bool, error) {
	if len(p.Companies) > 2 || hasDuplicate(p.Contacts) || hasDuplicate(p.Companies) {
		return false, nil
	}
	var contacts, companies int
	err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM contact WHERE id = ANY($1) AND archived_at IS NULL),
		       (SELECT count(*) FROM company WHERE id = ANY($2) AND archived_at IS NULL AND NOT is_anchor)`,
		p.Contacts, p.Companies).Scan(&contacts, &companies)
	if err != nil {
		return false, fmt.Errorf("capture: checking the hook's contacts and companies: %w", err)
	}
	return contacts == len(p.Contacts) && companies == len(p.Companies), nil
}

func hasDuplicate(list []ids.UUID) bool {
	seen := make(map[ids.UUID]bool, len(list))
	for _, id := range list {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// adjustVerdict runs the verdict hook at the end of the rule. It offers only a
// linked, ambiguous or empty verdict. Companies in conflict are not offered.
func (h MailHooks) adjustVerdict(ctx context.Context, tx pgx.Tx, facts dealFacts, verdict DealVerdict) (DealVerdict, error) {
	verdict.Hooks = append(slices.Clone(facts.hooks), verdict.Hooks...)
	if h.Verdict == nil || !h.active() || !slices.Contains([]DealOutcome{DealLinked, DealAmbiguous, DealNone}, verdict.Outcome) {
		return verdict, nil
	}
	view, ok, err := mailView(ctx, tx, facts)
	if err != nil || !ok {
		return verdict, err
	}
	out, changes := h.Verdict(view, verdict)
	if len(changes) == 0 {
		return verdict, nil
	}
	accepted, ok, err := acceptHookVerdict(ctx, tx, facts.activity, verdict, out)
	if err != nil || !ok {
		return verdict, err
	}
	accepted.Hooks = append(verdict.Hooks, changes...)
	return accepted, nil
}

// acceptHookVerdict keeps a hook's verdict only when core could have reached it
// and no colleague declined it. A hook can drop a deal, narrow a question or
// name a deal. It cannot raise a question the rule did not, add a candidate,
// or bring back a deal a colleague said the mail is not about.
func acceptHookVerdict(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, prev, out DealVerdict) (DealVerdict, bool, error) {
	switch out.Outcome {
	case DealNone:
		return DealVerdict{Outcome: DealNone, Basis: prev.Basis}, true, nil
	case DealLinked:
		if out.Deal.IsZero() {
			return prev, false, nil
		}
		declined, err := dealDeclined(ctx, tx, mail, out.Deal)
		if err != nil || declined {
			return prev, false, err
		}
		return DealVerdict{Outcome: DealLinked, Deal: out.Deal, Basis: DealBasisExtension, Candidates: []ids.UUID{out.Deal}}, true, nil
	case DealAmbiguous:
		if prev.Outcome != DealAmbiguous || len(out.Candidates) == 0 {
			return prev, false, nil
		}
		for _, candidate := range out.Candidates {
			if !slices.Contains(prev.Candidates, candidate) {
				return prev, false, nil
			}
		}
		kept, err := undeclinedDeals(ctx, tx, mail, out.Candidates)
		if err != nil || len(kept) == 0 {
			return prev, false, err
		}
		next := prev
		next.Candidates = kept
		return next, true, nil
	}
	return prev, false, nil
}

// withHookEvidence adds the extensions that shaped a verdict to the link's
// evidence, so the audit row names each unit and why it changed the result.
func withHookEvidence(evidence map[string]any, changes []HookChange) map[string]any {
	if len(changes) == 0 {
		return evidence
	}
	recorded := make([]map[string]string, len(changes))
	for i, c := range changes {
		recorded[i] = map[string]string{"unit": c.Unit, "hook": c.Hook, "reason": c.Reason}
	}
	out := make(map[string]any, len(evidence)+1)
	for k, v := range evidence {
		out[k] = v
	}
	out["extension_hooks"] = recorded
	return out
}

func (h MailHooks) active() bool { return h.Active == nil || h.Active() }
