// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
)

// knowledgeHandlers is the knowledge module's seat on Server, occupied before
// the module exists.
//
// It is here because Server carries a compile-time assertion that it satisfies
// the whole generated ServerInterface, so a contract operation with no method
// breaks the build rather than serving a 404 nobody can explain. The contract
// is written first on purpose — the handler signatures, request and response
// types are generated from it — which leaves exactly this window where the
// routes are declared and the module that answers them is not yet composed.
//
// Every operation answers 501 for that reason, and the reason is temporary:
// each method below is replaced by the real one as the module lands, and this
// type is gone when the last of them is. It holds no dependency because it has
// none to guard — an unwired dependency would be a field checked for nil, and
// there is nothing yet to wire.
type knowledgeHandlers struct{}

func (knowledgeHandlers) ListCorpora(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplemented(w, r, "listCorpora")
}

func (knowledgeHandlers) CreateCorpus(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplemented(w, r, "createCorpus")
}

func (knowledgeHandlers) ReadCorpus(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "readCorpus")
}

func (knowledgeHandlers) UpdateCorpus(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "updateCorpus")
}

func (knowledgeHandlers) ArchiveCorpus(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.NotImplemented(w, r, "archiveCorpus")
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
