// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newBulkEngine builds the bulk-change engine over its own module stores, the
// way the lifecycle seams build theirs. gate is the one whose volume meter an
// agent's changed records are admitted against.
func newBulkEngine(db *database.DB, gate *auth.Gate) *bulkEngine {
	return &bulkEngine{
		db:      db,
		targets: bulkTargets(contacts.NewStore(db), deals.NewStore(db, DealsInstallation())),
		gate:    gate,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// wireBulkSurface binds the bulk-change routes. Its gate takes the server's
// volume meter, the one the tool registry charges an agent's changed records
// to, so the change is admitted against the counter it is paid into.
func (s *Server) wireBulkSurface(pool *pgxpool.Pool) {
	s.bulkHandlers = bulkHandlers{engine: newBulkEngine(InstallationDB(pool),
		auth.NewGate(identity.NewService(pool), auth.WithVolumeMeter(s.volumeMeter)))}
}

// bulkHandlers is the REST door onto the bulk-change engine.
type bulkHandlers struct {
	engine *bulkEngine
}

func (h bulkHandlers) PreviewBulkChange(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.BulkChangePreviewRequest
	if err := httperr.DecodeOrRefusal(w, r, &body); err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.engine.Preview(r.Context(), bulkChange{
		recordType: body.RecordType, verb: body.Verb, items: body.Items, ownerID: bulkOwner(body.OwnerId),
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ExecuteBulkChange needs nothing of Idempotency-Key itself: the middleware
// claims the key and replays the first answer to a retry.
func (h bulkHandlers) ExecuteBulkChange(w http.ResponseWriter, r *http.Request, _ crmcontracts.ExecuteBulkChangeParams) {
	var body crmcontracts.BulkChangeExecuteRequest
	if err := httperr.DecodeOrRefusal(w, r, &body); err != nil {
		httperr.Write(w, r, err)
		return
	}
	change := bulkChange{
		recordType: body.RecordType, verb: body.Verb, items: body.Items, ownerID: bulkOwner(body.OwnerId),
	}
	if body.ConfirmToken != nil {
		change.confirmToken = *body.ConfirmToken
	}
	out, err := h.engine.Execute(r.Context(), change)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func bulkOwner(wire *openapi_types.UUID) *ids.UUID {
	if wire == nil {
		return nil
	}
	id := ids.UUID(*wire)
	return &id
}
