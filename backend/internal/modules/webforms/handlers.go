// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the module's transport slice: the admin surface over the store
// and the public intake over Intake.
type Handlers struct {
	store  *Store
	intake *Intake
	// now is the clock signed timestamps and the fill-time check read.
	now func() time.Time
	// throttle brakes the public intake and answers each form's intake state.
	throttle *Throttle
}

// WithThrottle brakes the public intake and shows each form's state on its read.
func (h Handlers) WithThrottle(t *Throttle) Handlers {
	h.throttle = t
	return h
}

// Throttle is the intake's brake, which the public edge asks before any work.
func (h Handlers) Throttle() *Throttle { return h.throttle }

// formChanged tells the throttle's memory of real form keys that key changed.
func (h Handlers) formChanged(key string) {
	if h.throttle != nil {
		h.throttle.FormChanged(key)
	}
}

// wireWithIntake is the form as its admin reads it, throttle state included.
func (h Handlers) wireWithIntake(ctx context.Context, f Form) crmcontracts.WebForm {
	w := WireForm(f)
	if h.throttle == nil {
		return w
	}
	state := h.throttle.IntakeState(ctx, f.Key)
	if state.State == "" {
		return w
	}
	w.Intake = &struct {
		Since *time.Time                      `json:"since,omitempty"`
		State crmcontracts.WebFormIntakeState `json:"state"`
		Until *time.Time                      `json:"until,omitempty"`
	}{Since: state.Since, State: crmcontracts.WebFormIntakeState(state.State), Until: state.Until}
	return w
}

// NewHandlers wires the transport.
func NewHandlers(store *Store, intake *Intake, now func() time.Time) Handlers {
	return Handlers{store: store, intake: intake, now: now}
}

// ListWebForms lists the workspace's forms.
func (h Handlers) ListWebForms(w http.ResponseWriter, r *http.Request, params crmcontracts.ListWebFormsParams) {
	archived := storekit.LiveOnly
	if params.IncludeArchived != nil && *params.IncludeArchived {
		archived = storekit.IncludeArchived
	}
	forms, err := h.store.ListForms(r.Context(), archived)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	data := make([]crmcontracts.WebForm, 0, len(forms))
	for _, f := range forms {
		data = append(data, h.wireWithIntake(r.Context(), f))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.WebFormListResponse{Data: data, SecretsAvailable: h.store.SecretsAvailable()})
}

// CreateWebForm registers a form; a server form's secret is in the answer once.
func (h Handlers) CreateWebForm(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateWebFormRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateInput{
		Name: req.Name, Sender: string(req.Sender), SenderSecret: req.SenderSecret, LeadOrigin: req.LeadOrigin,
		OwnerID: uuidOf(req.OwnerId), ProductID: uuidOf(req.ProductId), ConsentPolicyVersion: req.ConsentPolicyVersion,
		ConsentWording: req.ConsentWording, PrivacyNotice: req.PrivacyNotice, PrivacyNoticeURL: req.PrivacyNoticeUrl,
		RedirectURL: req.RedirectUrl, SuccessMessage: req.SuccessMessage,
	}
	if req.AllowedOrigins != nil {
		in.AllowedOrigins = *req.AllowedOrigins
	}
	if req.FieldMap != nil {
		in.FieldMap = *req.FieldMap
	}
	if req.ConsentRequired != nil {
		in.ConsentRequired = *req.ConsentRequired
	}
	f, secret, err := h.store.CreateForm(r.Context(), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.formChanged(f.Key)
	httperr.WriteJSON(w, http.StatusCreated, wireCreated(f, secret))
}

// GetWebForm answers one form, or 404 when it is archived or absent.
func (h Handlers) GetWebForm(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	f, err := h.store.GetForm(r.Context(), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.wireWithIntake(r.Context(), f))
}

// UpdateWebForm changes a form under the If-Match version.
func (h Handlers) UpdateWebForm(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateWebFormParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateWebFormRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := httperr.RequireAnyField(r); err != nil {
		httperr.Write(w, r, err)
		return
	}
	in := UpdateInput{
		Name: req.Name, AllowedOrigins: req.AllowedOrigins, SenderSecret: req.SenderSecret,
		LeadOrigin: req.LeadOrigin, OwnerID: uuidOf(req.OwnerId), ProductID: uuidOf(req.ProductId), FieldMap: req.FieldMap,
		ConsentRequired: req.ConsentRequired, ConsentPolicyVersion: req.ConsentPolicyVersion,
		ConsentWording: req.ConsentWording, PrivacyNotice: req.PrivacyNotice, PrivacyNoticeURL: req.PrivacyNoticeUrl,
		RedirectURL: req.RedirectUrl, SuccessMessage: req.SuccessMessage,
		HandlingAutomationID: uuidOf(req.HandlingAutomationId),
		Clear:                httperr.ClearedFields(r), IfVersion: ifVersion,
	}
	if req.State != nil {
		state := string(*req.State)
		in.State = &state
	}
	f, err := h.store.UpdateForm(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.formChanged(f.Key)
	httperr.WriteJSON(w, http.StatusOK, WireForm(f))
}

// ArchiveWebForm retires a form; its address answers 404 from then on.
func (h Handlers) ArchiveWebForm(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	f, err := h.store.ArchiveForm(r.Context(), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.formChanged(f.Key)
	httperr.WriteJSON(w, http.StatusOK, WireForm(f))
}

// RotateWebFormSecret mints a server form's new secret and answers it once.
func (h Handlers) RotateWebFormSecret(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	f, secret, err := h.store.RotateSecret(r.Context(), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireCreated(f, secret))
}

// WithFormSecrets binds the secret store to both halves, so the admin surface and
// the intake read secrets from one place.
func (h Handlers) WithFormSecrets(vault keyvault.Vault) Handlers {
	if h.store == nil {
		// A role that composed no forms surface has nothing to bind.
		return h
	}
	h.store = h.store.WithVault(vault)
	h.intake = NewIntake(h.store, h.intake.lander)
	return h
}
