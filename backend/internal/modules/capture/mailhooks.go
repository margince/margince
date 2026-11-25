// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The two places an extension may adjust the deal rule. Compose has already
// run every unit's hook in order, with its error and time limit. This file
// holds what core insists on afterwards. The result must be one core could have
// reached, and it must not undo what a colleague decided. A result that fails is
// dropped and the previous one stays.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// DealBasisExtension is the verdict of an extension that linked a deal. No topic
// check follows it, because the extension decided on its own rules.
const DealBasisExtension DealBasis = "extension"

// HookResult is what became of one hook's answer.
type HookResult string

// The results. A change that is neither failed nor ignored took effect.
const (
	HookChanged HookResult = "changed"
	HookFailed  HookResult = "failed"
	// HookIgnored is an answer core refused: it named no declared reason, or it
	// was not a result core could have reached.
	HookIgnored HookResult = "ignored"
)

// HookChange is one extension's change to a result, and its reason. Reason is
// the key the unit declared for it, or empty when the hook failed.
type HookChange struct {
	Unit   string
	Hook   string
	Reason string
	Result HookResult
}

// hookFailed is whether any hook failed. A failed hook decided nothing.
func hookFailed(changes []HookChange) bool {
	return slices.ContainsFunc(changes, func(c HookChange) bool { return c.Result == HookFailed })
}

// hookChanges are the changes that took effect, so a failed or ignored answer
// never reaches the audit evidence as if it had.
func hookChanges(changes []HookChange) []HookChange {
	return slices.DeleteFunc(slices.Clone(changes), func(c HookChange) bool {
		return c.Result == HookFailed || c.Result == HookIgnored
	})
}

// MailView is what a hook may read about one mail.
type MailView struct {
	Activity  ids.ActivityID
	Meeting   bool
	Subject   string
	Addresses []string
	// Body is the start of the text, read only where the seat may read it.
	Body string
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
	Parties func(context.Context, MailView, MailParties) (MailParties, []HookChange)
	Verdict func(context.Context, MailView, DealVerdict) (DealVerdict, []HookChange)
}

// mailView reads what a hook may see. It answers false when the deciding seat
// may not read the mail's content, so a held mail's subject never reaches a hook.
func mailView(ctx context.Context, tx pgx.Tx, facts dealFacts) (MailView, bool, error) {
	args := []any{facts.activity}
	arg := func(v any) int { args = append(args, v); return len(args) }
	bodyLimit := arg(extension.MailBodyCharacters)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return MailView{}, false, err
	}
	if scope == "" {
		scope = sqlUnscoped
	}
	view := MailView{Activity: facts.activity, Meeting: facts.meeting}
	err = tx.QueryRow(ctx, `
		SELECT coalesce(a.subject, ''), coalesce(left(a.body, $`+strconv.Itoa(bodyLimit)+`), ''),
		       coalesce((SELECT array_agg(DISTINCT lower(p.address) ORDER BY lower(p.address))
		                   FROM activity_participant p WHERE p.activity_id = a.id AND p.address IS NOT NULL), '{}')
		  FROM activity a WHERE a.id = $1 AND (`+scope+`)`, args...).Scan(&view.Subject, &view.Body, &view.Addresses)
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
	out, changes := h.Parties(ctx, view, MailParties{Contacts: facts.contacts, Companies: facts.companies})
	if hookFailed(changes) {
		facts.hooks = append(facts.hooks, changes...)
		facts.outcome = DealHookFailed
		return nil
	}
	if len(hookChanges(changes)) == 0 {
		facts.hooks = append(facts.hooks, changes...)
		return nil
	}
	valid, err := partiesAreLive(ctx, tx, out)
	if err != nil {
		return err
	}
	if !valid {
		facts.hooks = append(facts.hooks, asIgnored(changes)...)
		return nil
	}
	facts.contacts, facts.companies = out.Contacts, out.Companies
	facts.hooks = append(facts.hooks, changes...)
	return nil
}

// asIgnored marks changes core refused.
func asIgnored(changes []HookChange) []HookChange {
	out := slices.Clone(changes)
	for i := range out {
		out[i].Result = HookIgnored
	}
	return out
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
	if contacts != len(p.Contacts) || companies != len(p.Companies) {
		return false, nil
	}
	// Live is not enough: the mailbox owner must be able to see each one, as
	// they must for what core derives. A hook cannot file mail under a record
	// its owner may not read.
	if ok, err := allVisible(ctx, tx, "contact", p.Contacts); !ok || err != nil {
		return false, err
	}
	return allVisible(ctx, tx, "company", p.Companies)
}

// allVisible is whether the deciding seat sees every id of the table.
func allVisible(ctx context.Context, tx pgx.Tx, table string, rows []ids.UUID) (bool, error) {
	seen, err := auth.VisibleSubset(ctx, tx, table, rows)
	if err != nil {
		return false, err
	}
	for _, id := range rows {
		if !seen[id] {
			return false, nil
		}
	}
	return true, nil
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
func (h MailHooks) adjustVerdict(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates, verdict DealVerdict) (DealVerdict, error) {
	verdict.Hooks = append(slices.Clone(facts.hooks), verdict.Hooks...)
	if h.Verdict == nil || !h.active() || !slices.Contains([]DealOutcome{DealLinked, DealAmbiguous, DealNone}, verdict.Outcome) {
		return verdict, nil
	}
	view, ok, err := mailView(ctx, tx, facts)
	if err != nil || !ok {
		return verdict, err
	}
	out, changes := h.Verdict(ctx, view, verdict)
	if hookFailed(changes) {
		return DealVerdict{Outcome: DealHookFailed, Basis: verdict.Basis, Hooks: slices.Concat(verdict.Hooks, changes)}, nil
	}
	if len(hookChanges(changes)) == 0 {
		verdict.Hooks = slices.Concat(verdict.Hooks, changes)
		return verdict, nil
	}
	accepted, ok, err := acceptHookVerdict(ctx, tx, facts, candidates, verdict, out)
	if err != nil {
		return verdict, err
	}
	if !ok {
		verdict.Hooks = slices.Concat(verdict.Hooks, asIgnored(changes))
		return verdict, nil
	}
	accepted.Hooks = slices.Concat(verdict.Hooks, changes)
	return accepted, nil
}

// acceptHookVerdict keeps a hook's verdict only when core could have reached it
// and no colleague declined it. A hook can drop a deal, narrow a question or
// name a deal. It cannot raise a question the rule did not, add a candidate,
// or bring back a deal a colleague said the mail is not about.
func acceptHookVerdict(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates, prev, out DealVerdict) (DealVerdict, bool, error) {
	switch out.Outcome {
	case DealNone:
		return DealVerdict{Outcome: DealNone, Basis: prev.Basis}, true, nil
	case DealLinked:
		if out.Deal.IsZero() {
			return prev, false, nil
		}
		reached, err := dealReachable(ctx, tx, facts, candidates, prev, out.Deal)
		if err != nil || !reached {
			return prev, false, err
		}
		declined, err := dealDeclined(ctx, tx, facts.activity, out.Deal)
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
		kept, err := undeclinedDeals(ctx, tx, facts.activity, out.Candidates)
		if err != nil || len(kept) == 0 {
			return prev, false, err
		}
		next := prev
		next.Candidates = kept
		return next, true, nil
	}
	return prev, false, nil
}

// dealReachable is whether core could have named the deal. It was a candidate
// of the verdict, or it is open on the mail's project or one of its companies.
// A deal of a company the mail has nothing to do with is out of reach.
func dealReachable(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates, prev DealVerdict, deal ids.UUID) (bool, error) {
	if slices.Contains(prev.Candidates, deal) {
		return true, nil
	}
	if !facts.project.IsZero() {
		if open, err := candidates.IsOpenDealOfProject(ctx, tx, deal, facts.project); open || err != nil {
			return open, err
		}
	}
	if len(facts.companies) == 0 {
		return false, nil
	}
	return candidates.IsOpenDealOfCompanies(ctx, tx, deal, facts.companies)
}

// withHookEvidence adds the extensions that shaped a verdict to the link's
// evidence, so the audit row names each unit and why it changed the result.
func withHookEvidence(evidence map[string]any, changes []HookChange) map[string]any {
	if changes = hookChanges(changes); len(changes) == 0 {
		return evidence
	}
	recorded := make([]map[string]string, len(changes))
	for i, c := range changes {
		recorded[i] = map[string]string{"unit": c.Unit, "hook": c.Hook, fieldReason: c.Reason}
	}
	out := make(map[string]any, len(evidence)+1)
	maps.Copy(out, evidence)
	out["extension_hooks"] = recorded
	return out
}

func (h MailHooks) active() bool { return h.Active == nil || h.Active() }
