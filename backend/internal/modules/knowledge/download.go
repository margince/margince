// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// OpenForDownload streams a document's own bytes, with what a download needs to
// name the file.
//
// This is what a citation POINTS AT. An answer names the document a sentence
// rests on and quotes it; without a way to open the file the reader cannot see
// the quote in place, and a citation nobody can follow is a citation in name
// only. The RBAC migration already says as much — the document read grant is
// there so "the person who received a cited answer can open what it cited" —
// and this is the code that makes the sentence true.
//
// Gated on knowledge_document:read, which every seeded role holds, because the
// person who received the answer is the person who needs to check it.
func (s *Store) OpenForDownload(ctx context.Context, documentID ids.UUID) (crmcontracts.KnowledgeDocument, io.ReadCloser, error) {
	if err := auth.Require(ctx, "knowledge_document", principal.ActionRead); err != nil {
		return crmcontracts.KnowledgeDocument{}, nil, err
	}
	if s.blob == nil {
		return crmcontracts.KnowledgeDocument{}, nil, ErrBlobstoreUnconfigured
	}
	var doc crmcontracts.KnowledgeDocument
	var key string
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT storage_key FROM knowledge_document WHERE id = $1 AND archived_at IS NULL`,
			documentID).Scan(&key); err != nil {
			return notFoundOr(err, "read the document to download")
		}
		var rerr error
		doc, rerr = readDocument(ctx, tx, documentID)
		return rerr
	}); err != nil {
		return crmcontracts.KnowledgeDocument{}, nil, err
	}
	body, _, err := s.blob.Get(ctx, key)
	if err != nil {
		return crmcontracts.KnowledgeDocument{}, nil, fmt.Errorf("read the stored document: %w", err)
	}
	return doc, body, nil
}
