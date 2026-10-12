// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// documentColumns is the read shape over alias d. Its passage columns come from
// passageColumns, the corpus coverage's own, so a document and its corpus count alike.
var documentColumns = `d.id, d.corpus_id, d.filename, d.content_type, d.byte_size,
	d.ingest_status, d.ingest_detail, d.chunk_count, d.created_at, d.managed_source,
	` + ingestInFlight + `,
	(SELECT c.reindexing FROM knowledge_corpus c WHERE c.id = d.corpus_id),
	` + passageColumns("k.document_id = d.id")

func readDocument(ctx context.Context, tx pgx.Tx, id ids.UUID, identity string) (crmcontracts.KnowledgeDocument, error) {
	doc, err := scanDocument(tx.QueryRow(ctx,
		`SELECT `+documentColumns+` FROM knowledge_document d WHERE d.id = $1 AND d.archived_at IS NULL`, id), identity)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.KnowledgeDocument{}, apperrors.ErrNotFound
	}
	return doc, err
}

func scanDocument(row pgx.Row, identity string) (crmcontracts.KnowledgeDocument, error) {
	var doc crmcontracts.KnowledgeDocument
	var id, corpusID ids.UUID
	var status string
	var chunkCount int
	var managedSource *string
	var p passageCoverage
	if err := row.Scan(&id, &corpusID, &doc.Filename, &doc.ContentType, &doc.ByteSize,
		&status, &doc.IngestDetail, &chunkCount, &doc.CreatedAt, &managedSource,
		&p.inFlight, &p.reindexing, &p.total, &p.embedded, &p.identities); err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	doc.Id = openapi_types.UUID(id)
	doc.CorpusId = openapi_types.UUID(corpusID)
	doc.IngestStatus = crmcontracts.KnowledgeDocumentIngestStatus(status)
	doc.ChunkCount = &chunkCount
	doc.Shipped = managedSource != nil
	doc.Title = shippedTitle(managedSource, doc.Filename)
	doc.ChunksEmbedded = p.embedded
	doc.EmbedState = p.state(identity)
	return doc, nil
}

// shippedTitle is a handbook page's own heading. An uploaded document has none,
// and neither does a row whose page this build no longer ships.
func shippedTitle(managedSource *string, filename string) *string {
	if managedSource == nil || *managedSource != HandbookSource {
		return nil
	}
	title := handbook.Title(filename)
	if title == "" {
		return nil
	}
	return &title
}
