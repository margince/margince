// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// corpusSummary computes the meter over the non-excluded manifest.
func corpusSummary(ctx context.Context, tx pgx.Tx, profileID ids.UUID) (CorpusSummary, error) {
	summary := CorpusSummary{
		TargetWords:   CorpusTargetWords,
		RegisterWords: map[string]int{},
	}
	rows, err := tx.Query(ctx, `
		SELECT register, sum(word_count)::int, count(*)::int
		FROM voice_corpus_source
		WHERE voice_profile_id = $1 AND NOT excluded AND archived_at IS NULL AND content_erased_at IS NULL
		GROUP BY register`, profileID)
	if err != nil {
		return CorpusSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			register string
			words    int
			count    int
		)
		if err := rows.Scan(&register, &words, &count); err != nil {
			return CorpusSummary{}, err
		}
		summary.RegisterWords[register] = words
		summary.TotalWords += words
		summary.SourceCount += count
	}
	if err := rows.Err(); err != nil {
		return CorpusSummary{}, err
	}
	summary.QualityBand = QualityBand(summary.TotalWords)
	summary.Maturity = Maturity(summary.TotalWords)
	return summary, nil
}

// corpusSourceHash fingerprints the included corpus AS THE BUILDER SEES IT:
// content plus every builder-relevant manifest field (kind, register,
// weight), so a manifest edit — not just a content edit — reads as a
// changed corpus.
func corpusSourceHash(ctx context.Context, tx pgx.Tx, profileID ids.UUID) (string, error) {
	var hash string
	err := tx.QueryRow(ctx, `
		SELECT md5(coalesce(string_agg(
		         content_hash || ':' || kind || ':' || register || ':' || weight::text,
		         ',' ORDER BY source_ref), ''))
		FROM voice_corpus_source
		WHERE voice_profile_id = $1 AND NOT excluded AND archived_at IS NULL AND content_erased_at IS NULL`,
		profileID).Scan(&hash)
	return hash, err
}

// ProfilePresentation derives the two honest corpus axes and the pending
// candidate pointer without materializing them on the control row. The owner
// predicate is repeated even for an archived profile returned by DELETE.
func (s *VoiceStore) ProfilePresentation(ctx context.Context, profileID ids.UUID) (CorpusSummary, *int, error) {
	view, err := s.ProfileView(ctx, profileID)
	return view.Summary, view.CandidateVersion, err
}

// VoiceProfileView is what a profile read derives beside the control row: the
// corpus meter, the pending candidate and the newest build. Setup and the
// Writing voice page read their next step off these facts, so they are served
// together from one transaction and cannot disagree with each other.
type VoiceProfileView struct {
	Summary          CorpusSummary
	CandidateVersion *int
	// LatestBuild is the newest non-archived build, nil when none was queued.
	LatestBuild *VoiceBuild
}

// ProfileView reads the profile's derived presentation under the owner
// predicate ProfilePresentation documents.
func (s *VoiceStore) ProfileView(ctx context.Context, profileID ids.UUID) (VoiceProfileView, error) {
	if err := auth.Require(ctx, "voice_profile", principal.ActionRead); err != nil {
		return VoiceProfileView{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return VoiceProfileView{}, apperrors.ErrPermissionDenied
	}
	var view VoiceProfileView
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM voice_profile
			  WHERE id = $1 AND scope = 'user' AND owner_id = $2
			)`, profileID, actor.UserID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperrors.ErrNotFound
		}
		var err error
		view.Summary, err = corpusSummary(ctx, tx, profileID)
		if err != nil {
			return err
		}
		if view.CandidateVersion, err = pendingCandidateVersion(ctx, tx, profileID); err != nil {
			return err
		}
		view.LatestBuild, err = latestVoiceBuild(ctx, tx, profileID)
		return err
	})
	return view, err
}

// pendingCandidateVersion is the newest candidate awaiting review, or nil.
func pendingCandidateVersion(ctx context.Context, tx pgx.Tx, profileID ids.UUID) (*int, error) {
	var candidate int
	err := tx.QueryRow(ctx, `
		SELECT profile_version
		FROM voice_profile_version
		WHERE voice_profile_id = $1 AND status = 'candidate' AND archived_at IS NULL
		ORDER BY profile_version DESC LIMIT 1`, profileID).Scan(&candidate)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return &candidate, nil
	}
}

// latestVoiceBuild is the profile's newest non-archived build in any status,
// or nil. A build that outlived the page that started it is still found here.
func latestVoiceBuild(ctx context.Context, tx pgx.Tx, profileID ids.UUID) (*VoiceBuild, error) {
	build, err := scanVoiceBuild(tx.QueryRow(ctx, storekit.SQLf(`
		SELECT %s FROM voice_build
		WHERE voice_profile_id = $1 AND archived_at IS NULL
		ORDER BY created_at DESC, id DESC LIMIT 1`, voiceBuildColumns), profileID))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return &build, nil
	}
}

// lockProfileForCorpusWrite takes the voice_profile row a corpus mutation holds
// for the rest of its transaction, and it is the FIRST lock such a writer takes
// — before the voice_corpus_source row it is about to write.
//
// The order is the whole contract. RunBuild and CompleteBuild lock
// profile-then-build; a corpus writer locks profile-then-source. One order
// across all of them means no pair can wait on each other in opposite
// directions.
//
// Reaching the profile LAST is what deadlocked. Each ingest first took the
// advisory lock naming its own source (keyed on source_ref, so ingests of
// different files never met there) and inserted its row, and only then queued
// for this shared profile row. Onboarding drops several files at once, so
// several writers each held a different source and reached for this one in
// whatever order they arrived. Postgres broke the resulting cycles by aborting
// all but one with SQLSTATE 40P01, which reaches the reader as a 500 they can
// do nothing about: the corpus kept whichever ingest won and the rest were
// lost, with nothing on screen saying which.
//
// Taking it here rather than dropping it keeps what the lock is FOR. RunBuild
// and CompleteBuild each read corpusSourceHash while holding this row and
// refuse a build whose corpus moved underneath it. A corpus writer that did not
// hold it could commit a source after a build had read that hash, and the build
// would then publish a profile built from a corpus that no longer exists.
func lockProfileForCorpusWrite(ctx context.Context, tx pgx.Tx, profileID ids.UUID) error {
	_, err := storekit.LockRow(ctx, tx, "voice_profile", profileID, storekit.LiveOnly)
	return err
}

// markProfileStale records that the corpus moved under a built profile, so the
// next read knows the built voice no longer describes its sources. Every caller
// already holds this row through lockProfileForCorpusWrite; re-taking it is
// idempotent, and leaving it spelled out keeps the function readable alone.
func markProfileStale(ctx context.Context, tx pgx.Tx, profile VoiceProfile, now time.Time) error {
	if _, err := storekit.LockRow(ctx, tx, "voice_profile", profile.ID, storekit.LiveOnly); err != nil {
		return err
	}
	var (
		profileVersion int
		status         string
	)
	if err := tx.QueryRow(ctx, `
		SELECT profile_version, status FROM voice_profile WHERE id = $1`, profile.ID).Scan(&profileVersion, &status); err != nil {
		return err
	}
	if profileVersion == 0 || status == voiceProfileStatusStale {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE voice_profile
		SET status = $2, version = version + 1, updated_at = $3
		WHERE id = $1`, profile.ID, voiceProfileStatusStale, now)
	return err
}
