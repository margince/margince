// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The documents a legal review pack carries. Files belong to activities and
// the pack to deals. Neither imports the other, so the edge is made here.
//
// Read through the deal's own Files area, under the caller's principal. A
// file captured with a message follows that message's audience. So the pack
// holds what the requester could open, and shows a reviewer what they can.

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewDocumentCategories are the files legal reads: the contract, the legal
// paperwork and the offer. Everything else on the deal is left out.
var reviewDocumentCategories = map[crmcontracts.AttachmentCategory]bool{
	crmcontracts.AttachmentCategoryContract: true,
	crmcontracts.AttachmentCategoryLegal:    true,
	crmcontracts.AttachmentCategoryOffer:    true,
}

type legalReviewDocuments struct{ store *activities.Store }

// ReviewDocuments walks every page of the deal's Files area and keeps the
// contract, legal and offer documents that are not superseded.
func (d legalReviewDocuments) ReviewDocuments(ctx context.Context, dealID ids.UUID) ([]deals.PacketDocument, error) {
	var out []deals.PacketDocument
	var cursor *string
	for {
		page, next, err := d.store.ListDealDocuments(ctx, dealID, activities.DealDocumentFilters{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, doc := range page {
			if packed, ok := packetDocument(doc.Attachment); ok {
				packed.FromMessage = doc.Origin != nil
				out = append(out, packed)
			}
		}
		if !next.HasMore {
			return out, nil
		}
		cursor = &next.NextCursor
	}
}

func packetDocument(a crmcontracts.Attachment) (deals.PacketDocument, bool) {
	if a.Category == nil || !reviewDocumentCategories[*a.Category] {
		return deals.PacketDocument{}, false
	}
	state := crmcontracts.AttachmentDocStateCurrent
	if a.DocState != nil {
		state = *a.DocState
	}
	if state == crmcontracts.AttachmentDocStateSuperseded {
		return deals.PacketDocument{}, false
	}
	return deals.PacketDocument{
		AttachmentID: ids.UUID(a.Id), Category: string(*a.Category), DocState: string(state),
		ContentType: a.ContentType, ByteSize: a.ByteSize, AddedAt: a.CreatedAt,
	}, true
}

// OpenableDocuments asks the attachment's own read gate about each file in a
// pack, under the reader. A file filed with mail the reader may not read
// answers not found, and the pack leaves it out. So does a withheld file.
func (d legalReviewDocuments) OpenableDocuments(ctx context.Context, attachmentIDs []ids.UUID) (map[ids.UUID]deals.DocumentName, error) {
	out := make(map[ids.UUID]deals.DocumentName, len(attachmentIDs))
	for _, id := range attachmentIDs {
		meta, err := d.store.GetAttachmentMeta(ctx, id)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if meta.BytesWithheld != nil && *meta.BytesWithheld {
			continue
		}
		out[id] = deals.DocumentName{Title: meta.Title, Filename: meta.Filename}
	}
	return out, nil
}
