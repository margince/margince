// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sentMailExclusionErased is the reason a sample carries once the mail it was
// read from had its content erased. sentMailErasedLabel replaces its label,
// which held the mail's subject line.
const (
	sentMailExclusionErased = "source_erased"
	sentMailErasedLabel     = "Erased mail"
)

// sentMailScrubSQL clears what still names the mail on an emptied sent-mail
// sample: the label holding its subject line, and the hash of its text. The
// owner's remove and clear assign it beside their own erasure.
const sentMailScrubSQL = `source_label = CASE WHEN starts_with(source_ref, '` + sentMailRefPrefix + `')
	    THEN '` + sentMailErasedLabel + `' ELSE source_label END,
	  content_hash = CASE WHEN starts_with(source_ref, '` + sentMailRefPrefix + `') THEN '' ELSE content_hash END`

// sampleCopy is one voice sample erased with the mail it was copied from.
// included reads the reason the update kept: only a sample that fed the voice
// had none, and so takes the erased reason. fresh is a sample whose text this
// erasure emptied, rather than one the owner had already removed.
type sampleCopy struct {
	id, profileID    ids.UUID
	origin, register string
	words            int
	included, fresh  bool
}

// EraseSamplesCopiedFromTx empties every voice sample read from one of the
// given activities, in the caller's transaction. Such a sample is a copy of
// the mail's text, so it goes with the mail's content. The row stays as a
// removed source, so a later press does not learn the mail again. Every
// profile that held one is marked stale so it rebuilds.
//
// A sample the owner already removed is scrubbed too, since its label may
// still hold the subject line. Only a sample this call emptied is audited.
func EraseSamplesCopiedFromTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, now time.Time) error {
	if len(activityIDs) == 0 {
		return nil
	}
	refs := make([]string, 0, len(activityIDs))
	for _, id := range activityIDs {
		refs = append(refs, sentMailRefPrefix+id.String())
	}
	rows, err := tx.Query(ctx, `
		WITH was AS (
		  SELECT id, content_erased_at IS NULL AS fresh FROM voice_corpus_source
		   WHERE source_ref = ANY($1)
		     AND (content_erased_at IS NULL OR source_label <> $4 OR content_hash <> '')
		   FOR UPDATE
		)
		UPDATE voice_corpus_source v SET content = NULL, content_hash = '', source_label = $4,
		  content_erased_at = coalesce(v.content_erased_at, $2), archived_at = coalesce(v.archived_at, $2),
		  excluded = true, exclusion_reason = coalesce(v.exclusion_reason, $3),
		  version = v.version + 1, updated_at = $2
		  FROM was WHERE v.id = was.id
		 RETURNING v.id, v.voice_profile_id, v.origin, v.register, v.word_count,
		           v.exclusion_reason = $3, was.fresh`,
		refs, now, sentMailExclusionErased, sentMailErasedLabel)
	if err != nil {
		return err
	}
	var copies []sampleCopy
	var c sampleCopy
	if _, err := pgx.ForEachRow(rows, []any{&c.id, &c.profileID, &c.origin, &c.register, &c.words, &c.included, &c.fresh}, func() error {
		copies = append(copies, c)
		return nil
	}); err != nil {
		return err
	}
	for _, sample := range copies {
		if !sample.fresh {
			continue
		}
		if err := recordSampleErased(ctx, tx, sample, now); err != nil {
			return err
		}
	}
	return nil
}

// recordSampleErased writes the audit row and the corpus event for one erased
// sample, and marks its profile stale.
func recordSampleErased(ctx context.Context, tx pgx.Tx, sample sampleCopy, now time.Time) error {
	auditID, err := storekit.Audit(ctx, tx, "erase", "voice_corpus_source", sample.id,
		map[string]any{voiceColumnWordCount: sample.words, voiceKeyIncluded: sample.included}, nil)
	if err != nil {
		return err
	}
	if err := markProfileStale(ctx, tx, VoiceProfile{ID: sample.profileID}, now); err != nil {
		return err
	}
	summary, err := corpusSummary(ctx, tx, sample.profileID)
	if err != nil {
		return err
	}
	hash, err := corpusSourceHash(ctx, tx, sample.profileID)
	if err != nil {
		return err
	}
	wordDelta := 0
	if sample.included {
		wordDelta = -sample.words
	}
	return storekit.EmitEvent(ctx, tx, auditID, sample.profileID,
		voiceCorpusChangedPayload(sample.profileID, &sample.id, "removed", &sample.origin, &sample.register,
			wordDelta, summary.SourceCount, hash))
}
