// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// sentMailExclusionErased is the reason a sample carries once the mail it was
// read from had its content erased. sentMailErasedLabel replaces its label,
// which held the mail's subject line.
const (
	sentMailExclusionErased = "source_erased"
	sentMailErasedLabel     = "Erased mail"
)

// sampleExclusionSubjectErased is the reason a sample carries once a contact it
// named was erased, and sampleSubjectErasedLabel replaces its label.
const (
	sampleExclusionSubjectErased = "subject_erased"
	sampleSubjectErasedLabel     = "Erased sample"
)

// sentMailScrubSQL clears what still names the mail on an emptied sent-mail
// sample: the label holding its subject line, and the hash of its text. The
// owner's remove and clear assign it beside their own erasure.
const sentMailScrubSQL = `source_label = CASE WHEN starts_with(source_ref, '` + sentMailRefPrefix + `')
	    THEN '` + sentMailErasedLabel + `' ELSE source_label END,
	  content_hash = CASE WHEN starts_with(source_ref, '` + sentMailRefPrefix + `') THEN '' ELSE content_hash END`

// sampleCopy is one voice sample an erasure emptied.
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
	return eraseSamplesTx(ctx, tx, sampleErasure{
		match:  `source_ref = ANY($1) AND ` + sampleNotScrubbedSQL,
		arg:    refs,
		reason: sentMailExclusionErased,
		label:  sentMailErasedLabel,
	}, now)
}

// EraseSamplesNamingTx empties every voice sample whose text or label names a
// contact being erased: one of their addresses, or their full name as whole
// words. Any sample can name them, uploaded and pasted ones included. None of
// those links to their record, so the words are the only way to find it.
// The whole sample goes, because its hash and word count describe the text.
func EraseSamplesNamingTx(ctx context.Context, tx pgx.Tx, fullName string, emails []string, now time.Time) error {
	patterns := make([]string, 0, len(emails)+1)
	for _, email := range emails {
		if email = strings.TrimSpace(email); email != "" {
			patterns = append(patterns, wholeAddressPattern(email))
		}
	}
	if pattern, ok := wholeNamePattern(fullName); ok {
		patterns = append(patterns, pattern)
	}
	if len(patterns) == 0 {
		return nil
	}
	return eraseSamplesTx(ctx, tx, sampleErasure{
		match:  `(content ~* ANY($1) OR source_label ~* ANY($1)) AND ` + sampleNotScrubbedSQL,
		arg:    patterns,
		reason: sampleExclusionSubjectErased,
		label:  sampleSubjectErasedLabel,
	}, now)
}

// wholeAddressPattern is the regular expression an address is found by. It
// is bounded only by characters that would make it a different address: joann@
// is not ann@, and ann@example.com.invalid is not ann@example.com. Any other
// mark beside it, such as markdown or brackets, still matches. A missed
// erasure costs more than one extra emptied sample.
func wholeAddressPattern(email string) string {
	return values.EmailTextPattern(email)
}

// wholeNamePattern is the regular expression a full name is found by: every
// word of it, whole and in order, with any spacing between them. A one-word
// name fits too many contacts to erase by. "Anna" would empty every sample
// greeting another Anna.
func wholeNamePattern(fullName string) (string, bool) {
	return values.FullNameTextPattern(fullName, 2)
}

// sampleNotScrubbedSQL keeps a sample this erasure already finished out of
// its match, so a replay changes nothing.
const sampleNotScrubbedSQL = `(content_erased_at IS NULL OR source_label <> $2 OR content_hash <> '')`

// sampleErasure says which samples one erasure empties and what they carry
// after it. match is a literal predicate over $1 (arg) and $2 (label).
type sampleErasure struct {
	match         string
	arg           []string
	reason, label string
}

// eraseSamplesTx empties the matched samples, keeps each row as a removed
// source, and audits and reports the ones it emptied.
//
// The profiles are locked first, in id order, then the sources. That is the
// order every corpus writer takes (lockProfileForCorpusWrite), so an erasure
// never waits on an ingest that waits on it.
func eraseSamplesTx(ctx context.Context, tx pgx.Tx, erasure sampleErasure, now time.Time) error {
	found, err := tx.Query(ctx, `
		SELECT DISTINCT voice_profile_id FROM voice_corpus_source
		 WHERE `+erasure.match+`
		 ORDER BY voice_profile_id`, erasure.arg, erasure.label)
	if err != nil {
		return err
	}
	profiles, err := pgx.CollectRows(found, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return nil
	}
	for _, profile := range profiles {
		if _, err := storekit.LockRow(ctx, tx, "voice_profile", profile, storekit.IncludeArchived); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `
		WITH was AS (
		  SELECT id, content_erased_at IS NULL AS fresh FROM voice_corpus_source
		   WHERE voice_profile_id = ANY($5) AND `+erasure.match+`
		   FOR UPDATE
		)
		UPDATE voice_corpus_source v SET content = NULL, content_hash = '', source_label = $2,
		  content_erased_at = coalesce(v.content_erased_at, $3), archived_at = coalesce(v.archived_at, $3),
		  excluded = true, exclusion_reason = coalesce(v.exclusion_reason, $4),
		  version = v.version + 1, updated_at = $3
		  FROM was WHERE v.id = was.id
		 RETURNING v.id, v.voice_profile_id, v.origin, v.register, v.word_count,
		           v.exclusion_reason = $4, was.fresh`,
		erasure.arg, erasure.label, now, erasure.reason, profiles)
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
