// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The REST door's half of the commands with a second path operand beyond the
// route's own {id}. They act on a company fact or profile field, or on a
// project stakeholder. Each parses {id} as the existence-hiding
// 404 (routedID), decodes the second operand, and hands the typed command to
// its resolver (modules/agents/commandsidecar.go, commandaction.go).

import (
	"encoding/json"
	"net/http"

	chi "github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pathOperand reads a required path parameter beyond the route's own {id}.
// chi has already matched the route by the time a handler runs, so an empty
// value means the parameter was never bound — a request built by hand
// (tests) or a future route missing the segment — and answers 422 naming the
// parameter, not a panic on an empty FactKey/Field later.
func pathOperand(r *http.Request, name string) (string, error) {
	v := chi.URLParam(r, name)
	if v == "" {
		return "", httperr.Validation(name, "missing", name+" is required")
	}
	return v, nil
}

// routedID parses the route's own {id}, the existence-hiding answer every
// restCommands decoder gives a malformed one: "that is not a uuid" and
// "there is no such row" must read alike, or the shape of a caller's id
// tells them which rows exist.
func routedID(r *http.Request) (ids.UUID, error) {
	id, err := ids.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return ids.UUID{}, apperrors.ErrNotFound
	}
	return id, nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func confirmFactCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	factKey, err := pathOperand(r, "factKey")
	if err != nil {
		return nil, err
	}
	return agents.NewConfirmFactCall(deps.records, deps.language, agents.ConfirmFactCommand{ID: id, FactKey: factKey}), nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func updateFactCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	factKey, err := pathOperand(r, "factKey")
	if err != nil {
		return nil, err
	}
	return agents.NewUpdateFactCall(deps.records, deps.language, agents.UpdateFactCommand{ID: id, FactKey: factKey}), nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func createFactCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	return agents.NewCreateFactCall(deps.records, deps.language, agents.CreateFactCommand{ID: id}), nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func deleteFactCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	factKey, err := pathOperand(r, "factKey")
	if err != nil {
		return nil, err
	}
	return agents.NewDeleteFactCall(deps.records, deps.language, agents.DeleteFactCommand{ID: id, FactKey: factKey}), nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func confirmProfileFieldCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	field, err := pathOperand(r, "field")
	if err != nil {
		return nil, err
	}
	return agents.NewConfirmProfileFieldCall(deps.records, deps.language, agents.ConfirmProfileFieldCommand{ID: id, Field: field}), nil
}

//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func updateProfileFieldCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	field, err := pathOperand(r, "field")
	if err != nil {
		return nil, err
	}
	return agents.NewUpdateProfileFieldCall(deps.records, deps.language, agents.UpdateProfileFieldCommand{ID: id, Field: field}), nil
}

// setStakeholderCommand decodes PUT /v1/projects/{id}/stakeholders, whose
// contact_id arrives in the BODY where its DELETE twin carries it in the path.
//
// The shape check is the same one removeStakeholderCommand makes for its own
// operand, and for the same reason: the request the approval stages IS the
// request its redemption replays, so a contact_id that names no contact is a call
// the handler refuses AFTER a human's one-shot approval has been consumed. 422
// rather than the routed id's 404 — like the path operand, contact_id says WHICH
// edge, not whether the project exists.
//
// It is checked here rather than in the resolver because there is nothing for
// the command to carry it as: neither Guards nor Subject reads the attached
// contact (setStakeholderResolver's own doc says why), and a field with no
// reader documents no obligation.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func setStakeholderCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	if err := requireStakeholderContact(body); err != nil {
		return nil, err
	}
	return agents.NewSetStakeholderCall(deps.records, deps.language, agents.SetStakeholderCommand{ID: id}), nil
}

// requireStakeholderContact holds the body to the one member the attach cannot
// run without. crm.yaml's SetProjectStakeholderRequest requires contact_id as a
// uuid; a body that is not even an object is answered as the same missing
// contact_id, since neither carries one.
func requireStakeholderContact(body []byte) error {
	var payload struct {
		ContactID string `json:"contact_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.ContactID == "" {
		return httperr.Validation("contact_id", "missing", "contact_id is required")
	}
	if _, err := ids.Parse(payload.ContactID); err != nil {
		return httperr.Validation("contact_id", "invalid", "contact_id must be a uuid")
	}
	return nil
}

// removeStakeholderCommand decodes DELETE /v1/projects/{id}/stakeholders/{contact_id}.
// contact_id fails as 422, not 404: unlike the routed {id}, a malformed or
// missing contact_id names no row this door hides the existence of — it
// names which edge the caller meant, a shape the caller simply got wrong.
// Composed from the same pathOperand every other second-operand decoder
// uses (a missing segment answers "missing") plus ids.Parse for the shape
// check the others don't need (a non-empty but malformed one answers
// "invalid") — one spelling of "required", not a second copy of it.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func removeStakeholderCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	raw, err := pathOperand(r, "contact_id")
	if err != nil {
		return nil, err
	}
	contactID, perr := ids.Parse(raw)
	if perr != nil {
		return nil, httperr.Validation("contact_id", "invalid", "contact_id must be a uuid")
	}
	return agents.NewRemoveStakeholderCall(deps.records, deps.language, agents.RemoveStakeholderCommand{ID: id, ContactID: contactID}), nil
}

// createRoomItemCommand decodes POST /v1/deal-rooms/{id}/documents and the
// thread-opening and reply POSTs through the same code: the routed room is
// folded into the body so the staged approval names where the item lands, and
// the policy's record type says which kind of item it is.
//
// The routed {id} names the ROOM, and the record being created is the item, so
// this cannot be the plain createCommand: that one carries only the body, and an
// approval of it would say nothing about which room the item lands in. The room
// travels in the fields, where the staged proposed_change shows it beside the
// wording a human is being asked to release.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func createRoomItemCommand(pol agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error) {
	roomID, err := routedID(r)
	if err != nil {
		return nil, err
	}
	parents := map[string]ids.UUID{"deal_room_id": roomID}
	if pol.RecordType == recordTypeDealRoomComment {
		raw, err := pathOperand(r, "threadId")
		if err != nil {
			return nil, err
		}
		threadID, perr := ids.Parse(raw)
		if perr != nil {
			return nil, apperrors.ErrNotFound
		}
		parents["thread_id"] = threadID
	}
	fields, err := withParents(body, parents)
	if err != nil {
		return nil, err
	}
	return agents.NewCreateCall(deps.language, agents.CreateCommand{
		RecordType: string(pol.RecordType),
		Fields:     fields,
	}), nil
}

// withParents folds the routed parents into a create body, under the names a
// create_record call gives them. The staged approval names where the item
// lands, and both doors stage one command. A body that is not an object is
// refused here: a create whose fields cannot carry its room belongs nowhere.
func withParents(body []byte, parents map[string]ids.UUID) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &fields); err != nil {
			return nil, httperr.Validation("body", "invalid", "the request body must be a JSON object")
		}
	}
	for name, id := range parents {
		encoded, err := json.Marshal(id)
		if err != nil {
			return nil, httperr.Validation(name, "invalid", "the "+name+" in the path is not a uuid")
		}
		fields[name] = encoded
	}
	merged, err := json.Marshal(fields)
	if err != nil {
		return nil, httperr.Validation("body", "invalid", "the request body could not be read as fields")
	}
	return merged, nil
}

// setCompanyCommand decodes PUT /v1/projects/{id}/companies. The body's
// company_id is held here for the same reason the stakeholder attach holds
// contact_id: an attach that names no company cannot run, and refusing at
// staging tells the caller that rather than staging an approval that will fail
// when it is redeemed.
//
//nolint:ireturn // the call IS the product: a concrete resolver here is exactly the thing that must not leave the agents package
func setCompanyCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	if err := requireCompanyCompany(body); err != nil {
		return nil, err
	}
	return agents.NewSetCompanyCall(deps.records, deps.language, agents.SetCompanyCommand{ID: id}), nil
}

// requireCompanyCompany holds the body to the one member the attach cannot
// run without. A body that is not even an object is answered as the same
// missing company_id, since neither carries one.
func requireCompanyCompany(body []byte) error {
	var payload struct {
		CompanyID string `json:"company_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.CompanyID == "" {
		return httperr.Validation("company_id", "missing", "company_id is required")
	}
	if _, err := ids.Parse(payload.CompanyID); err != nil {
		return httperr.Validation("company_id", "invalid", "company_id must be a uuid")
	}
	return nil
}

// removeCompanyCommand decodes DELETE
// /v1/projects/{id}/companies/{company_id} — the company is a second PATH
// operand, so it is read from the route rather than a body.
//
//nolint:ireturn // the call IS the product: a concrete resolver here is exactly the thing that must not leave the agents package
func removeCompanyCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	id, err := routedID(r)
	if err != nil {
		return nil, err
	}
	raw, err := pathOperand(r, "company_id")
	if err != nil {
		return nil, err
	}
	companyID, perr := ids.Parse(raw)
	if perr != nil {
		return nil, httperr.Validation("company_id", "invalid", "company_id must be a uuid")
	}
	return agents.NewRemoveCompanyCall(deps.records, deps.language,
		agents.RemoveCompanyCommand{ID: id, CompanyID: companyID}), nil
}
