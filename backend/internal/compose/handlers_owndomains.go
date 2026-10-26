// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The workspace own-domain surface (CAP-WIRE-2a, ADR-0082/A127): list the
// domains this installation treats as its own, register one, remove one. Thin
// transport — the capture store owns the RBAC gate and the audited write.

import (
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type ownDomainHandlers struct {
	store *capture.OwnDomainStore
	// evidence, when set, judges again the stage evidence a changed domain
	// can move, right after the change. The hourly evidence_authorship pass
	// is the backstop when it fails.
	evidence *deals.Store
	log      *slog.Logger
}

// reconcileEvidenceFor judges again the stage evidence written from the
// domain. The domain change has committed, so a failure here is logged, and
// the hourly pass corrects the rows.
func (h ownDomainHandlers) reconcileEvidenceFor(r *http.Request, domain string) {
	if h.evidence == nil {
		return
	}
	ctx := principal.SystemActing(r.Context(), evidenceAuthorshipActor)
	if _, err := h.evidence.ReconcileEvidenceAuthorship(ctx, ownDomainReader{store: h.store},
		deals.EvidenceAuthorshipScope{Domain: domain}); err != nil && h.log != nil {
		h.log.WarnContext(r.Context(), "own domains: judging stage evidence again failed, the hourly pass retries", "err", err)
	}
}

func (h ownDomainHandlers) ListWorkspaceEmailDomains(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ListWorkspaceEmailDomains")
		return
	}
	list, err := h.store.List(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	resp := crmcontracts.WorkspaceEmailDomainListResponse{
		Data: make([]crmcontracts.WorkspaceEmailDomain, 0, len(list.Domains)),
	}
	for _, d := range list.Domains {
		resp.Data = append(resp.Data, toContractOwnDomain(d))
	}
	if len(list.AnchorDomains) > 0 {
		claimed := list.AnchorDomains
		resp.AnchorDomains = &claimed
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

func (h ownDomainHandlers) CreateWorkspaceEmailDomain(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "CreateWorkspaceEmailDomain")
		return
	}
	// Human-only (x-agent-access). The set decides what the CRM may hold, so an
	// agent must not widen it; the store re-checks the admin/ops grant.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.CreateWorkspaceEmailDomainRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// Vetted here so a bad domain answers 422 naming what is wrong with it,
	// rather than reaching the store and surfacing as an opaque fault.
	if _, err := capture.ValidOwnDomain(req.Domain); err != nil {
		httperr.Write(w, r, httperr.Validation("domain", "invalid_domain", err.Error()))
		return
	}
	domain, err := h.store.Add(r.Context(), req.Domain)
	if errors.Is(err, capture.ErrConsumerMailDomain) {
		httperr.Write(w, r, httperr.Validation("domain", "consumer_mail_domain", err.Error()))
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.reconcileEvidenceFor(r, domain.Domain)
	httperr.WriteJSON(w, http.StatusCreated, toContractOwnDomain(domain))
}

func (h ownDomainHandlers) DeleteWorkspaceEmailDomain(w http.ResponseWriter, r *http.Request, domain string) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "DeleteWorkspaceEmailDomain")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := h.store.Remove(r.Context(), domain); err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.reconcileEvidenceFor(r, domain)
	w.WriteHeader(http.StatusNoContent)
}

// toContractOwnDomain maps one registered domain onto the wire shape.
func toContractOwnDomain(d capture.OwnDomain) crmcontracts.WorkspaceEmailDomain {
	out := crmcontracts.WorkspaceEmailDomain{
		Domain:   d.Domain,
		Source:   crmcontracts.WorkspaceEmailDomainSource(d.Source),
		Verified: d.Verified,
	}
	if !d.CreatedAt.IsZero() {
		created := d.CreatedAt
		out.CreatedAt = &created
	}
	return out
}
