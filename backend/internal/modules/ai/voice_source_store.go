// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IngestSource runs the §B1 pipeline (normalize → speaker-filter →
// register-tag → count) and upserts the manifest row by source_ref:
// re-ingesting a source replaces it — the meter never double-counts —
// and re-adding an excluded source is an explicit opt back in.
func (s *VoiceStore) IngestSource(ctx context.Context, profileID ids.UUID, in IngestSourceInput) (VoiceCorpusSource, CorpusSummary, CorpusIngestStats, error) {
	if err := auth.Require(ctx, "voice_profile", principal.ActionUpdate); err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, CorpusIngestStats{}, err
	}
	prepared, err := prepareSource(in)
	if err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, CorpusIngestStats{}, err
	}

	var (
		source  VoiceCorpusSource
		summary CorpusSummary
	)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		source, summary, err = s.ingestPreparedSource(ctx, tx, profileID, prepared)
		return err
	})
	if err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, CorpusIngestStats{}, err
	}
	return source, summary, prepared.Stats, nil
}

// PreviewSource dry-runs one candidate source under the owner gate:
// detected shape and per-speaker word counts, nothing stored.
func (s *VoiceStore) PreviewSource(ctx context.Context, profileID ids.UUID, format, content string) (CorpusPreview, error) {
	if err := auth.Require(ctx, "voice_profile", principal.ActionUpdate); err != nil {
		return CorpusPreview{}, err
	}
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := s.visibleProfile(ctx, tx, profileID)
		return err
	}); err != nil {
		return CorpusPreview{}, err
	}
	return PreviewCorpusText(format, content)
}

func (s *VoiceStore) ingestPreparedSource(ctx context.Context, tx pgx.Tx, profileID ids.UUID, prepared preparedSource) (VoiceCorpusSource, CorpusSummary, error) {
	// Before the source row this is about to write, so every corpus writer
	// queues here in one order — see lockProfileForCorpusWrite.
	if err := lockProfileForCorpusWrite(ctx, tx, profileID); err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, err
	}
	profile, err := s.visibleProfile(ctx, tx, profileID)
	if err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, err
	}
	if err := ownerOnly(ctx, profile); err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return VoiceCorpusSource{}, CorpusSummary{}, apperrors.ErrPermissionDenied
	}
	source, prior, err := s.persistPreparedSource(ctx, tx, profileID, prepared, actor.ID)
	if err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, err
	}
	summary, err := s.recordSourceIngest(ctx, tx, profile, source, prior)
	if err != nil {
		return VoiceCorpusSource{}, CorpusSummary{}, err
	}
	return source, summary, nil
}

// voiceSourceWriteIdentity names the lock every writer of one corpus source
// takes: the pair that identifies it, not the profile, so two ingests of
// different sources under one profile do not serialize against each other.
const voiceSourceWriteIdentity = "voice_corpus_source"

// priorVoiceSource is the manifest row a re-ingest replaces — the fields the
// upsert overwrites, carried out of that same statement, so the audit row can
// say what the replaced source held. exists separates "there was no such
// source" from a source whose every field happened to read as a zero value.
type priorVoiceSource struct {
	kind      string
	register  string
	wordCount int
	excluded  bool
	exists    bool
}

// persistPreparedSource writes the source and reports the row it replaced, read
// by the upsert's own CTE. A separate read before it would be a different look
// at the table: a concurrent ingest of the same source_ref committing in between
// would leave this write recording a create for a row it actually replaced.
func (s *VoiceStore) persistPreparedSource(ctx context.Context, tx pgx.Tx, profileID ids.UUID, prepared preparedSource, actorID string) (VoiceCorpusSource, priorVoiceSource, error) {
	// The source's identity, held for the rest of the transaction. The CTE reads
	// the statement's own snapshot, and under READ COMMITTED the ON CONFLICT
	// re-check resolves against a row a concurrent ingest committed after it —
	// so without this, a replacement can report itself as a first ingest. The
	// domain writers beside this one take the same lock for the same reason.
	if err := storekit.LockWriteIdentity(ctx, tx, voiceSourceWriteIdentity, profileID.String()+" "+prepared.SourceRef); err != nil {
		return VoiceCorpusSource{}, priorVoiceSource{}, err
	}
	occurredAt := prepared.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.now().UTC()
	}
	var sourceID ids.UUID
	row := tx.QueryRow(ctx, `
			WITH was AS (
			  SELECT kind, register, word_count, excluded
			    FROM voice_corpus_source
			   WHERE voice_profile_id = $1 AND source_ref = $6 AND archived_at IS NULL
			)
			INSERT INTO voice_corpus_source
			  (voice_profile_id, origin, kind, register, weight, source_label,
			   source_ref, content, content_hash, word_count, excluded, exclusion_reason,
			   extractor_version, occurred_at, source, captured_by, updated_at)
			VALUES ($1, 'manual', $2, $3, $4, $5, $6, $7, $8, $9,
			        false, NULL, 'voice-v1', $10, 'manual', $11, $12)
			ON CONFLICT (voice_profile_id, source_ref) DO UPDATE SET
			  origin = 'manual',
			  kind = EXCLUDED.kind,
			  register = EXCLUDED.register,
			  weight = EXCLUDED.weight,
			  source_label = EXCLUDED.source_label,
			  content = EXCLUDED.content,
			  content_hash = EXCLUDED.content_hash,
			  word_count = EXCLUDED.word_count,
			  excluded = false,
			  exclusion_reason = NULL,
			  extractor_version = EXCLUDED.extractor_version,
			  occurred_at = EXCLUDED.occurred_at,
			  content_erased_at = NULL,
			  archived_at = NULL,
			  version = voice_corpus_source.version + 1,
			  updated_at = EXCLUDED.updated_at
			RETURNING id,
			          (SELECT was.kind FROM was), (SELECT was.register FROM was),
			          (SELECT was.word_count FROM was), (SELECT was.excluded FROM was)`,
		profileID, prepared.Kind, prepared.Register, prepared.Weight, prepared.Label,
		prepared.SourceRef, prepared.Text, SourceRefForContent(prepared.Text), prepared.Words,
		occurredAt, actorID, s.now().UTC())
	var prior priorVoiceSource
	var kind, register *string
	var wordCount *int
	var excluded *bool
	if err := row.Scan(&sourceID, &kind, &register, &wordCount, &excluded); err != nil {
		return VoiceCorpusSource{}, priorVoiceSource{}, err
	}
	if kind != nil {
		prior = priorVoiceSource{
			kind: *kind, register: *register, wordCount: *wordCount, excluded: *excluded, exists: true,
		}
	}
	source, err := scanVoiceSource(tx.QueryRow(ctx, storekit.SQLf(
		`SELECT %s FROM voice_corpus_source WHERE id = $1`, voiceSourceColumns), sourceID))
	return source, prior, err
}

func (s *VoiceStore) recordSourceIngest(ctx context.Context, tx pgx.Tx, profile VoiceProfile, source VoiceCorpusSource, prior priorVoiceSource) (CorpusSummary, error) {
	auditAction, eventAction, wordDelta := sourceIngestChange(source, prior)
	auditID, err := storekit.Audit(ctx, tx, auditAction, "voice_corpus_source", source.ID,
		priorSourceImage(profile, source, prior), map[string]any{
			voiceColumnProfileID: profile.ID, voiceKeyKind: source.Kind, voiceKeyRegister: source.Register,
			voiceKeySourceRef: source.SourceRef, voiceColumnWordCount: source.WordCount,
			voiceKeyExcluded: source.Excluded,
		})
	if err != nil {
		return CorpusSummary{}, err
	}
	summary, err := corpusSummary(ctx, tx, profile.ID)
	if err != nil {
		return CorpusSummary{}, err
	}
	sourceHash, err := corpusSourceHash(ctx, tx, profile.ID)
	if err != nil {
		return CorpusSummary{}, err
	}
	if err := markProfileStale(ctx, tx, profile, s.now().UTC()); err != nil {
		return CorpusSummary{}, err
	}
	err = storekit.EmitEvent(ctx, tx, auditID, profile.ID,
		voiceCorpusChangedPayload(profile.ID, &source.ID, eventAction, &source.Origin, &source.Register,
			wordDelta, summary.SourceCount, sourceHash))
	if err != nil {
		return CorpusSummary{}, err
	}
	return summary, nil
}

// priorSourceImage is the replaced source as the after image describes the new
// one — the same keys, so a reader diffs field against field. A first ingest
// replaced nothing and records no image; re-ingesting an excluded source is an
// explicit opt back in, which is a transition only this pair can show.
//
// The identity keys are carried unchanged because they are what the upsert
// conflicts on: a re-ingest is by definition the same profile and source_ref.
func priorSourceImage(profile VoiceProfile, source VoiceCorpusSource, prior priorVoiceSource) map[string]any {
	if !prior.exists {
		return nil
	}
	return map[string]any{
		voiceColumnProfileID: profile.ID, voiceKeyKind: prior.kind, voiceKeyRegister: prior.register,
		voiceKeySourceRef: source.SourceRef, voiceColumnWordCount: prior.wordCount,
		voiceKeyExcluded: prior.excluded,
	}
}

func sourceIngestChange(source VoiceCorpusSource, prior priorVoiceSource) (string, string, int) {
	if !prior.exists {
		return "create", "ingested", source.WordCount
	}
	wordDelta := source.WordCount
	if !prior.excluded {
		wordDelta -= prior.wordCount
	}
	return "update", "replaced", wordDelta
}

// ListSources returns the corpus manifest + the live meter for one
// visible profile (features/09 §B1.4 — the onboarding/voice.html read).
func (s *VoiceStore) ListSources(ctx context.Context, profileID ids.UUID) ([]VoiceCorpusSource, CorpusSummary, error) {
	if err := auth.Require(ctx, "voice_profile", principal.ActionRead); err != nil {
		return nil, CorpusSummary{}, err
	}
	var (
		sources []VoiceCorpusSource
		summary CorpusSummary
	)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		p, err := s.visibleProfile(ctx, tx, profileID)
		if err != nil {
			return err
		}
		if err := ownerOnly(ctx, p); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, storekit.SQLf(
			`SELECT %s FROM voice_corpus_source
			 WHERE voice_profile_id = $1 AND archived_at IS NULL
			 ORDER BY created_at DESC, id DESC`,
			voiceSourceColumns), profileID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			src, err := scanVoiceSource(rows)
			if err != nil {
				return err
			}
			sources = append(sources, src)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		summary, err = corpusSummary(ctx, tx, profileID)
		return err
	})
	if err != nil {
		return nil, CorpusSummary{}, err
	}
	return sources, summary, nil
}
