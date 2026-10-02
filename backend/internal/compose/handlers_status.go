// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetStatus implements (GET /status): the anonymous reachability probe.
//
// It does no work on purpose. What it proves is the public path into this
// process; dependency health belongs to /readyz, which takes an instance that
// lost a dependency out of rotation — and then this probe fails at the edge.
// A dependency read here as well would make an anonymous route cost a pool
// acquisition per request to answer a question readiness already owns.
//
// The session middleware in front of every /v1 route still resolves the
// installation workspace, but from an in-process cache once the first lookup
// has succeeded; before bootstrap that read answers 503, which is the one
// state this probe is meant to report through it.
func (s Server) GetStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ServiceStatus{Status: crmcontracts.ServiceStatusStatusOk})
}
