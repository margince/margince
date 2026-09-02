// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The vocabulary-management transport: read one word with its weight, rename
// it, restore it, fold two into one. Split from handlers.go for the file
// ceiling, and because these four are the admin surface where tags.go's
// apply/remove pair is every seat's.

// GetTag serves one word with how much of the workspace carries it.
func (h Handlers) GetTag(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	tag, usage, err := h.store.GetTag(r.Context(), pathID[ids.TagKind](id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireTagDetail(tag, usage))
}

// UpdateTag renames, recolours or describes a tag, under If-Match.
func (h Handlers) UpdateTag(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateTagParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateTagRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var expected int64
	if ifVersion != nil {
		expected = *ifVersion
	}
	tag, err := h.store.UpdateTag(r.Context(), pathID[ids.TagKind](id), tagUpdateFrom(req), expected)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireTag(tag))
}

// RestoreTag brings an archived word back into the vocabulary.
func (h Handlers) RestoreTag(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	tag, err := h.store.RestoreTag(r.Context(), pathID[ids.TagKind](id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireTag(tag))
}

// MergeTags folds this tag into another and reports what moved.
func (h Handlers) MergeTags(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.MergeTagsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	result, err := h.store.MergeTags(r.Context(),
		pathID[ids.TagKind](id), ids.From[ids.TagKind](ids.UUID(req.IntoTagId)))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.MergeTagsResult{
		IntoTagId: req.IntoTagId,
		Moved:     result.Moved,
		Collapsed: result.Collapsed,
	})
}

// tagUpdateFrom translates the wire partial into the store's.
//
// The store's clearable fields carry two levels of pointer: the outer says
// whether the caller mentioned the field, the inner whether a value survives.
// The wire cannot carry that distinction — an absent field and a null decode
// identically — so the sentinel values do it instead: "none" clears a colour,
// an empty string clears a description.
func tagUpdateFrom(req crmcontracts.UpdateTagRequest) TagUpdate {
	out := TagUpdate{Name: req.Name}
	if req.Color != nil {
		var color *string
		if *req.Color != clearColor {
			c := string(*req.Color)
			color = &c
		}
		out.Color = &color
	}
	if req.Description != nil {
		var description *string
		if *req.Description != "" {
			description = req.Description
		}
		out.Description = &description
	}
	return out
}

// clearColor is the value that removes a colour rather than setting one. It is
// not a tone the design system draws, which is what lets it mean "no tone"
// without colliding with one.
const clearColor = crmcontracts.UpdateTagRequestColorNone

func wireTagDetail(t tagRow, usage TagUsage) crmcontracts.TagDetail {
	var color *crmcontracts.TagDetailColor
	if t.Color != nil {
		c := crmcontracts.TagDetailColor(*t.Color)
		color = &c
	}
	version := t.Version
	return crmcontracts.TagDetail{
		Id:          openapi_types.UUID(t.ID.UUID),
		Name:        t.Name,
		Color:       color,
		Description: t.Description,
		Version:     &version,
		CreatedAt:   &t.CreatedAt,
		UpdatedAt:   &t.UpdatedAt,
		ArchivedAt:  t.ArchivedAt,
		Usage: crmcontracts.TagUsage{
			People:    usage.People,
			Companies: usage.Companies,
			Deals:     usage.Deals,
		},
	}
}

// The wire mappers for the tag surface, moved here from tags.go when that
// file reached its line ceiling: a mapper is transport, and this is the
// transport file.

func wireTag(t tagRow) crmcontracts.Tag {
	var color *crmcontracts.TagColor
	if t.Color != nil {
		c := crmcontracts.TagColor(*t.Color)
		color = &c
	}
	version := t.Version
	return crmcontracts.Tag{
		Id:          openapi_types.UUID(t.ID.UUID),
		Name:        t.Name,
		Color:       color,
		Description: t.Description,
		Version:     &version,
		CreatedAt:   &t.CreatedAt,
		UpdatedAt:   &t.UpdatedAt,
		ArchivedAt:  t.ArchivedAt,
	}
}

func wireTaggable(tg taggableRow) crmcontracts.Taggable {
	return crmcontracts.Taggable{
		Id:         openapi_types.UUID(tg.ID),
		TagId:      openapi_types.UUID(tg.TagID.UUID),
		EntityType: crmcontracts.TaggableEntityType(tg.EntityType),
		EntityId:   openapi_types.UUID(tg.EntityID),
		CreatedAt:  &tg.CreatedAt,
	}
}
