// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The rep's own standing overnight authority.
//
// It lives beside the passport handlers because granting IS minting: what
// authorizes a nightly run is a passport, and the single production mint binds
// on_behalf_of and granted_by to the same session user. The answer itself is a
// row another module owns, which arrives through the agentgrant port so that
// both halves commit in ONE transaction — see that package for why they must.
//
// WHOSE ANSWER IT IS. Nothing here takes a user id. The rep is the session
// user, and the port reads the acting principal, so the only answer any caller
// can record is their own. An admin cannot answer on somebody's behalf, which
// is the entire reason this is separate from the workspace setting that turns
// the feature on at all.

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/agentgrant"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// overnightPassportLabel names the credential a standing grant mints, so a rep
// reading their passport list can tell what created it and why it is there.
const overnightPassportLabel = "overnight brief"

// grantScopes is what each agent's credential may do with the rep's authority.
//
// PER AGENT, not one list for all of them, because they need different things
// and the difference is not cosmetic: `write` is all-or-nothing across twelve
// verbs, so a shared list either denies an agent a scope it needs or hands
// every agent one it does not. The catalog's own tool allowlist narrows
// further, but a scope not granted here cannot be regained by any narrowing.
//
// A scope missing here does NOT fail loudly. The runner degrades a run whose
// declared tool is not funded by its passport, so an under-scoped credential
// buys a nightly agent that starts, finds it cannot do its job, and stops —
// silently, at 2am. The gate below derives the requirement from the tool specs
// themselves, so this map cannot drift from what the agents actually call.
//
// Held by: TestEveryGrantFundsTheToolsItsAgentDeclares (backend/gates/agentgrantscopes_test.go)
var grantScopes = map[string][]string{
	// Reads deals and activities, ranks them, and writes its findings back onto
	// the run through annotate_brief — which is a write, so the credential has
	// to fund one. Without it every run degrades before the first step and the
	// rep gets a ranked queue with no sentence saying why anything is on it.
	"morning_brief": {"read", "write"},
	// Reads the same, and logs ONE note activity per at-risk deal — which is
	// log_activity, which requires write.
	"overnight_at_risk_sweep": {"read", "write"},
}

// scopesForAgent is what to mint for one agent, or false when this build does
// not know the agent — a credential with no declared scopes would be a
// passport that funds nothing, which is worse than refusing.
func scopesForAgent(spec string) ([]string, bool) {
	scopes, known := grantScopes[spec]
	return scopes, known
}

// ListMyAgentGrants implements (GET /me/agent-grants).
//
// It enumerates the CATALOG rather than the stored rows, because "never asked"
// is an answer the product must be able to give and no row records it. A client
// that could not tell a decline from an unasked question would ask the
// declining rep again every night, which is the one thing the stored decline
// exists to prevent.
func (h Handlers) ListMyAgentGrants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := identityFrom(ctx); !ok {
		httperr.Unauthorized(w, r, "a standing grant is answered by a signed-in human, not an agent")
		return
	}
	if h.agentGrants == nil {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	out := make([]crmcontracts.MyAgentGrant, 0, len(h.grantableAgents))
	// ONE transaction for every agent, not one each: these answers are shown
	// together as the rep's standing posture, and read separately they can come
	// from different moments — a revoke landing mid-list would render a page in
	// which one agent's credential is live and another's is not, describing a
	// state that never existed.
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, spec := range h.grantableAgents {
			answer, found, err := h.agentGrants.MyAnswerTx(ctx, tx, spec)
			if err != nil {
				return err
			}
			out = append(out, renderAgentGrant(spec, answer, found))
		}
		return nil
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.MyAgentGrants{Data: out})
}

// SetMyAgentGrant implements (PUT /me/agent-grants/{spec}): the rep answers.
func (h Handlers) SetMyAgentGrant(w http.ResponseWriter, r *http.Request, spec crmcontracts.ScheduledAgentName) {
	id, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "a standing grant is answered by a signed-in human, not an agent")
		return
	}
	if h.agentGrants == nil {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if !h.grantable(string(spec)) {
		httperr.Write(w, r, httperr.Validation("spec", "unknown_agent",
			fmt.Sprintf("no scheduled agent is named %q", spec)))
		return
	}
	var req crmcontracts.SetMyAgentGrantRequest
	if !httperr.Decode(w, r, &req) {
		return
	}

	answer, found, err := h.answerAgentGrant(r, id, string(spec), req.Granted)
	if err != nil {
		var badScope *InvalidScopeError
		if errors.As(err, &badScope) {
			httperr.Write(w, r, httperr.Validation("scopes", "invalid_scope", badScope.Error()))
			return
		}
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, renderAgentGrant(string(spec), answer, found))
}

// answerAgentGrant writes the answer and the credential together, then reads
// the result back inside the SAME transaction — so what the rep is shown is
// what committed, rather than a second read that a concurrent revoke could
// have changed underneath.
//
// GRANTING revokes the previous answer's passport first. Re-granting would
// otherwise leave the old credential live and unreferenced, which is authority
// nobody can find in order to end it.
//
// DECLINING revokes what the answer named. Withdrawing the reference without
// ending the credential would leave the agent able to act after the rep said
// stop, which is the one outcome a withdrawal must not have.
func (h Handlers) answerAgentGrant(
	r *http.Request, id Identity, spec string, granted bool,
) (agentgrant.Answer, bool, error) {
	var answer agentgrant.Answer
	var found bool
	ctx := actorCtx(r.Context(), id)
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		previous, hadAnswer, err := h.agentGrants.MyAnswerTx(ctx, tx, spec)
		if err != nil {
			return err
		}
		if hadAnswer && previous.PassportID != nil {
			if err := h.svc.RevokePassportTx(ctx, tx, id, *previous.PassportID); err != nil {
				return fmt.Errorf("end the authority the previous answer named: %w", err)
			}
		}
		state := agentgrant.StateDeclined
		var passportID *ids.PassportID
		if granted {
			scopes, known := scopesForAgent(spec)
			if !known {
				return fmt.Errorf("no scopes are declared for agent %q", spec)
			}
			label := overnightPassportLabel
			issued, err := IssuePassportTx(ctx, tx, id, IssuePassportInput{
				Label:  &label,
				Scopes: scopes,
			})
			if err != nil {
				return err
			}
			minted := ids.From[ids.PassportKind](issued.ID.UUID)
			state, passportID = agentgrant.StateGranted, &minted
		}
		if err := h.agentGrants.RecordAnswerTx(ctx, tx, spec, state, passportID); err != nil {
			return err
		}
		answer, found, err = h.agentGrants.MyAnswerTx(ctx, tx, spec)
		return err
	})
	return answer, found, err
}

// grantable reports whether a name is one this build actually schedules.
func (h Handlers) grantable(spec string) bool {
	for _, known := range h.grantableAgents {
		if known == spec {
			return true
		}
	}
	return false
}

// renderAgentGrant renders one answer, including the one that has no row.
//
// A granted answer whose credential is not usable is reported as
// granted-and-unusable rather than as a decline: the rep DID agree, and telling
// them otherwise would put a question to them they have already answered. That
// is the renewal case, and the client offers it as such.
func renderAgentGrant(spec string, answer agentgrant.Answer, found bool) crmcontracts.MyAgentGrant {
	out := crmcontracts.MyAgentGrant{
		Spec:  crmcontracts.ScheduledAgentName(spec),
		State: crmcontracts.MyAgentGrantStateNeverAsked,
	}
	if !found {
		return out
	}
	out.CredentialUsable = answer.CredentialUsable
	decided := answer.DecidedAt
	out.DecidedAt = &decided
	switch answer.State {
	case agentgrant.StateGranted:
		out.State = crmcontracts.MyAgentGrantStateGranted
	case agentgrant.StateDeclined:
		out.State = crmcontracts.MyAgentGrantStateDeclined
	default:
		// A state this build cannot spell is not rendered as an answer the rep
		// gave. Reporting it as never_asked asks them again, which is wrong but
		// recoverable; reporting it as granted would run an agent overnight on
		// the strength of a row nothing here wrote.
		out.State = crmcontracts.MyAgentGrantStateNeverAsked
	}
	return out
}
