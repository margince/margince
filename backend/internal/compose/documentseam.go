// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The document seam: attach_document writes through the store the human upload
// writes through, and list_documents reads what each record's Documents tab reads.

import (
	"bytes"
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// documentSeam reads the object store and the upload limit off srv per call,
// because both arrive by option after the registry is built (see importSeam).
// With no srv it lists but stores nothing: the store refuses with ErrBlobstoreUnconfigured.
type documentSeam struct {
	srv *Server
	db  *database.DB
}

func (d documentSeam) store() *activities.Store {
	var blob blobstore.Store
	if d.srv != nil {
		blob = d.srv.blob
	}
	return activities.NewStore(d.db).WithBlobstore(blob)
}

func (d documentSeam) Attach(ctx context.Context, upload agents.DocumentUpload) (agents.AttachedDocument, error) {
	stored, err := d.store().UploadAttachment(ctx, activities.AttachmentInput{
		EntityType: upload.EntityType, EntityID: upload.EntityID,
		Filename: upload.Filename, ContentType: upload.ContentType,
		Content: bytes.NewReader(upload.Content),
	})
	if err != nil {
		return agents.AttachedDocument{}, err
	}
	return attachedDocument(stored), nil
}

// List reads the company and deal libraries their tabs read, with the tabs'
// default filters; every other record's tab reads its own attachments.
func (d documentSeam) List(
	ctx context.Context, record agents.RecordLink, cursor string, limit int,
) (agents.DocumentPage, error) {
	var cursorArg *string
	if cursor != "" {
		cursorArg = &cursor
	}
	var limitArg *int
	if limit > 0 {
		limitArg = &limit
	}
	store := d.store()
	var (
		found []crmcontracts.Attachment
		page  storekit.Page
		err   error
	)
	switch datasource.EntityType(record.EntityType) {
	case datasource.EntityCompany:
		found, page, err = store.ListCompanyDocuments(ctx, record.EntityID,
			activities.DocumentFilters{Cursor: cursorArg, Limit: limitArg})
	case datasource.EntityDeal:
		var dealDocs []crmcontracts.DealDocument
		dealDocs, page, err = store.ListDealDocuments(ctx, record.EntityID,
			activities.DealDocumentFilters{Cursor: cursorArg, Limit: limitArg})
		for _, doc := range dealDocs {
			found = append(found, doc.Attachment)
		}
	default:
		found, page, err = store.ListAttachments(ctx, record.EntityType, record.EntityID, cursorArg, limitArg)
	}
	if err != nil {
		return agents.DocumentPage{}, err
	}
	out := agents.DocumentPage{Documents: make([]agents.AttachedDocument, 0, len(found))}
	for _, att := range found {
		out.Documents = append(out.Documents, attachedDocument(att))
	}
	if page.HasMore {
		out.NextCursor = page.NextCursor
	}
	return out, nil
}

// MaxBytes is the operator's attachment ceiling, the one the REST upload rides.
func (d documentSeam) MaxBytes(context.Context) int64 {
	if d.srv != nil {
		return d.srv.uploadLimits.Attachment
	}
	return deployconfig.Config{}.EffectiveUploads().Attachment
}

// attachedDocument is the wire row without what an agent has no use for; the
// storage key is never on the contract row to begin with.
func attachedDocument(att crmcontracts.Attachment) agents.AttachedDocument {
	return agents.AttachedDocument{
		RecordLink:   agents.RecordLink{EntityType: string(att.EntityType), EntityID: ids.UUID(att.EntityId)},
		AttachmentID: ids.UUID(att.Id),
		Filename:     att.Filename,
		ContentType:  orZero(att.ContentType),
		ByteSize:     orZero(att.ByteSize),
		Checksum:     orZero(att.Checksum),
		CapturedBy:   orZero(att.CapturedBy),
		CreatedAt:    att.CreatedAt,
	}
}

func orZero[T any](field *T) T {
	if field == nil {
		var zero T
		return zero
	}
	return *field
}
