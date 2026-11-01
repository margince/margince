// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func wireForm(f Form) crmcontracts.WebForm {
	fieldMap := f.FieldMap
	if fieldMap == nil {
		fieldMap = map[string]string{}
	}
	origins := f.AllowedOrigins
	if origins == nil {
		origins = []string{}
	}
	return crmcontracts.WebForm{
		Id:                   openapi_types.UUID(f.ID),
		Name:                 f.Name,
		FormKey:              f.Key,
		Sender:               crmcontracts.WebFormSender(f.Sender),
		AllowedOrigins:       origins,
		SecretConfigured:     f.SecretConfigured(),
		LeadOrigin:           f.LeadOrigin,
		OwnerId:              wireUUID(f.OwnerID),
		FieldMap:             fieldMap,
		ConsentRequired:      f.ConsentRequired,
		ConsentPolicyVersion: f.ConsentPolicyVersion,
		ConsentWording:       f.ConsentWording,
		PrivacyNotice:        f.PrivacyNotice,
		PrivacyNoticeUrl:     f.PrivacyNoticeURL,
		RedirectUrl:          f.RedirectURL,
		SuccessMessage:       f.SuccessMessage,
		State:                crmcontracts.WebFormState(f.State),
		Version:              f.Version,
		CreatedAt:            f.CreatedAt,
		UpdatedAt:            f.UpdatedAt,
		ArchivedAt:           f.ArchivedAt,
		LastPostAt:           f.LastPostAt,
	}
}

// wireCreated is the form plus a server form's secret, which the caller sees
// this once.
func wireCreated(f Form, secret string) crmcontracts.WebFormCreated {
	w := wireForm(f)
	out := crmcontracts.WebFormCreated{
		Id: w.Id, Name: w.Name, FormKey: w.FormKey, Sender: w.Sender, AllowedOrigins: w.AllowedOrigins,
		SecretConfigured: w.SecretConfigured, LeadOrigin: w.LeadOrigin, OwnerId: w.OwnerId, FieldMap: w.FieldMap,
		ConsentRequired: w.ConsentRequired, ConsentPolicyVersion: w.ConsentPolicyVersion,
		ConsentWording: w.ConsentWording, PrivacyNotice: w.PrivacyNotice, PrivacyNoticeUrl: w.PrivacyNoticeUrl,
		RedirectUrl: w.RedirectUrl, SuccessMessage: w.SuccessMessage,
		State: crmcontracts.WebFormCreatedState(w.State), Version: w.Version,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, ArchivedAt: w.ArchivedAt,
	}
	if secret != "" {
		out.SigningSecret = &secret
	}
	return out
}

func wireUUID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	v := openapi_types.UUID(*id)
	return &v
}

func uuidOf(id *openapi_types.UUID) *ids.UUID {
	if id == nil {
		return nil
	}
	v := ids.UUID(*id)
	return &v
}

// refusal states the module's own errors in the wire taxonomy. A bad field is
// a 422 naming it. A signature that did not verify is one opaque 401. A
// website not on the list is a 403, and a paused form a 410.
func refusal(err error) error {
	if bad, ok := errors.AsType[*BadInputError](err); ok {
		return httperr.Validation(bad.Field, "invalid", bad.Reason)
	}
	switch {
	case errors.Is(err, errUnverified):
		return &httperr.DetailedError{Status: http.StatusUnauthorized, Code: "unauthorized", Detail: "the submission's signature did not verify"}
	case errors.Is(err, errForeignOrigin):
		return &httperr.DetailedError{
			Status: http.StatusForbidden, Code: "origin_not_allowed",
			Detail: "this form does not take posts from this website; add it to the form's websites in Settings",
		}
	case errors.Is(err, ErrPaused):
		return &httperr.DetailedError{Status: http.StatusGone, Code: "form_paused", Detail: "this form is not taking submissions right now"}
	case errors.Is(err, errUnreadable), errors.Is(err, errBodyTooLarge):
		return httperr.Validation("body", "unreadable", err.Error())
	}
	return err
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	httperr.Write(w, r, refusal(err))
}
