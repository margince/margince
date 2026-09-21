// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The HTTP transport for the contact record page. Wire concerns only: bind
// the path id, refuse the modes this read cannot honestly serve, and hand
// the result to the sentinel error mapping. The service owns the
// transaction and every gate.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entityTypeContact is this baseline's record type; migration 0184 widened
// the table's CHECK to admit it beside company.
const entityTypeContact = "contact"

// Handlers shadows the generated contact-360 stubs.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetContact360 implements GET /contacts/{id}/360.
func (h Handlers) GetContact360(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetContact360Params) {
	var opts AssembleOptions
	if params.ProjectId != nil {
		opts.ProjectID = ptr(ids.From[ids.ProjectKind](ids.UUID(*params.ProjectId)))
	}
	view, err := h.svc.AssembleScoped(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)), opts)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, view)
}

// AcknowledgeContactView implements POST /contacts/{id}/view-ack.
func (h Handlers) AcknowledgeContactView(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	ack, err := h.svc.Acknowledge(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, ack)
}

// GetContactProfileFields implements GET /contacts/{id}/profile-fields.
func (h Handlers) GetContactProfileFields(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	fields, err := h.svc.ProfileFields(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.ContactProfileField `json:"data"`
	}{Data: fields})
}

// Acknowledge records that the calling human has now seen this contact.
//
// The upsert takes GREATEST(stored, now), so a slow tab's late-arriving ack
// can never rewind a newer one.
//
// The human gate is load-bearing, not defense in depth: an agent principal
// carries the granting human's id as its UserID, so resolving "the acting
// user" would happily mark a record as SEEN by a human who never opened it,
// consuming their unread marker on their behalf.
func (s *Service) Acknowledge(ctx context.Context, contactID ids.ContactID) (crmcontracts.RecordViewAck, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.RecordViewAck{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.RecordViewAck{}, err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return crmcontracts.RecordViewAck{}, err
	}
	now := s.now().UTC()
	var stored time.Time
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Anything that names a record is gated: acknowledging a contact the
		// caller cannot read would confirm they exist.
		// Live, not merely visible: Art. 17 anonymizes a contact in place and
		// stamps archived_at while LEAVING owner_id alone, so the plain probe
		// still admits their owner. This sidecar is the only path to the
		// enrichment rows — the 360 refuses through its LiveOnly root read —
		// and it would otherwise serve the subject's name, title and employer
		// after the controller certified them erased.
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		// company360's writer, not a copy of its statement: it owns
		// user_record_view (tableownership_test.go names it), and the upsert's
		// GREATEST is the whole correctness argument. The gate ABOVE is this
		// package's own, because that is the part that legitimately differs.
		stored, err = company360.RecordVisit(ctx, tx, userID, entityTypeContact, contactID.UUID, now)
		return err
	})
	if err != nil {
		return crmcontracts.RecordViewAck{}, err
	}
	return crmcontracts.RecordViewAck{
		EntityType:   crmcontracts.RecordViewAckEntityTypeContact,
		EntityId:     openapi_types.UUID(contactID.UUID),
		LastViewedAt: stored,
	}, nil
}

// ProfileFields serves the enrichment evidence sidecar on its own. The
// contact read is the gate: evidence about a contact the caller cannot see
// is not disclosed.
func (s *Service) ProfileFields(ctx context.Context, contactID ids.ContactID) ([]crmcontracts.ContactProfileField, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.ContactProfileField
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Live, not merely visible: Art. 17 anonymizes a contact in place and
		// stamps archived_at while LEAVING owner_id alone, so the plain probe
		// still admits their owner. This sidecar is the only path to the
		// enrichment rows — the 360 refuses through its LiveOnly root read —
		// and it would otherwise serve the subject's name, title and employer
		// after the controller certified them erased.
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		fields, err := s.readProfileFields(ctx, tx, contactID)
		if err != nil {
			return err
		}
		out = fields
		return nil
	})
	return out, err
}
