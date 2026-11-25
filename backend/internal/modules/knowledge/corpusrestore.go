// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RestoreCorpus brings an archived corpus back with its documents and their
// chunks. Single documents are deleted rather than archived, so every archived
// document of the corpus was archived with it and comes back with it.
//
// It does not make the corpus the default again. The flag moved elsewhere when
// it was archived, and a restore says nothing about which set to ask first.
// Restoring a live corpus answers it unchanged, with no second audit row.
func (s *Store) RestoreCorpus(ctx context.Context, id ids.UUID) (crmcontracts.KnowledgeCorpus, error) {
	if err := auth.Require(ctx, "knowledge_corpus", principal.ActionDelete); err != nil {
		return crmcontracts.KnowledgeCorpus{}, err
	}
	var out crmcontracts.KnowledgeCorpus
	err := s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readCorpus(ctx, tx, id, storekit.IncludeArchived)
		if err != nil {
			return err
		}
		if current.ArchivedAt() == nil {
			out = current.wire(s.embedIdentity())
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_corpus SET archived_at = NULL, default_ask = false
			 WHERE id = $1 AND archived_at IS NOT NULL`, id); err != nil {
			return storekit.NameTaken(fmt.Errorf("restore knowledge corpus: %w", err), corpusEntry, current.corpus.Name, corpusNameIndex)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_document SET archived_at = NULL
			 WHERE corpus_id = $1 AND archived_at IS NOT NULL`, id); err != nil {
			return fmt.Errorf("restore the corpus's documents: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_chunk SET archived_at = NULL
			 WHERE corpus_id = $1 AND archived_at IS NOT NULL`, id); err != nil {
			return fmt.Errorf("restore the corpus's chunks: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "restore", "knowledge_corpus", id,
			map[string]any{defaultAskColumn: current.corpus.DefaultAsk},
			map[string]any{defaultAskColumn: false}); err != nil {
			return fmt.Errorf("audit knowledge corpus restore: %w", err)
		}
		restored, err := readCorpus(ctx, tx, id, storekit.LiveOnly)
		out = restored.wire(s.embedIdentity())
		return err
	})
	return out, err
}
