// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
)

// knowledgeHandlers is the knowledge module's seat on Server: the module's own
// transport for the operations it answers today, and a 501 for the ones whose
// module code has not landed yet.
//
// The stubs are here because Server carries a compile-time assertion that it
// satisfies the whole generated ServerInterface, so a contract operation with
// no method breaks the build rather than serving a 404 nobody can explain. The
// contract is written first on purpose — the handler signatures, request and
// response types are generated from it — which leaves exactly this window where
// a route is declared and the code answering it is not yet written.
//
// Each remaining stub is deleted by the change that implements its operation,
// and the embedded module surface grows to match. When the last one goes, this
// type is a bare embed and goes with it.
type knowledgeHandlers struct {
	knowledge.Handlers
}

func (knowledgeHandlers) ListCorpusDocuments(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "listCorpusDocuments")
}

func (knowledgeHandlers) DeleteCorpusDocument(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "deleteCorpusDocument")
}

func (knowledgeHandlers) AskCorpus(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "askCorpus")
}
