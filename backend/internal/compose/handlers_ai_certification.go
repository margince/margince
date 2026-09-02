// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification surface: how well the models this installation is bound to
// perform each AI job.
//
// Thin transport, like the binding handler beside it. The ai store owns the
// RBAC gate — the same `ai_routing` object, because a seat that may not see
// which models are bound has no use for how those models score — and
// certificationView owns the join.

import (
	"net/http"
	"sync"

	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// certificationInputs are the two committed trees this surface reads: the
// snapshot of measured results and the census of sites that should have one.
//
// Loaded once. Both are build artifacts — the snapshot is embedded, and the
// census is assembled from the compiled task contract — so re-deriving them per
// request would buy nothing and pay for it on every page load. A failure is
// carried rather than swallowed: it means the binary shipped a snapshot it
// cannot read, which the caller must be told about instead of being shown an
// empty page that looks like an installation with nothing measured.
var certificationInputs = sync.OnceValues(func() (struct {
	snap  snapshot.Snapshot
	sites []aitasks.Site
}, error,
) {
	var out struct {
		snap  snapshot.Snapshot
		sites []aitasks.Site
	}
	snap, err := snapshot.Load()
	if err != nil {
		return out, err
	}
	census, err := NewTaskCensus()
	if err != nil {
		return out, err
	}
	out.snap, out.sites = snap, census.All()
	return out, nil
})

func (h aiRoutingHandlers) GetAiCertification(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiCertification")
		return
	}
	// Human-only (x-agent-access), matching the binding this reports on: an
	// agent has no business reading which vendor the installation's
	// correspondence is trusted to, or how well it does it.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The STORED binding, never a seed file: seeds.ai_routing bootstraps a fresh
	// install once, and a running workspace's real binding lives in the database
	// and moves through this same settings page.
	//
	// Read BEFORE the build artifacts, because the store owns the RBAC gate: a
	// caller without ai_routing:read must meet a 403 whatever state the embedded
	// snapshot is in, and loading first would let a corrupt artifact answer an
	// unauthorized caller with a 500 instead.
	routing, err := h.store.Get(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	inputs, err := certificationInputs()
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, certificationView(routing, inputs.sites, inputs.snap))
}
