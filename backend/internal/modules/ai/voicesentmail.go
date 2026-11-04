// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SentMail is one mail the signed-in user sent from their own mailbox. Ref
// names the mail stably, so the same mail read twice is one source.
type SentMail struct {
	Ref     string
	Subject string
	Body    string
	SentAt  time.Time
}

// SentMailReader reads the newest limit mails the signed-in user sent
// themselves, locked in tx so none of them narrows before the press files it.
// Only the request handler is given one, so no background job can read mail
// into a voice.
type SentMailReader func(ctx context.Context, tx pgx.Tx, limit int) ([]SentMail, error)

// SentMailLearned is what one press of "learn from my sent mail" did. Read
// counts the mails the reader returned; each of them is added, already a
// source, or carried too few words of the user's own.
type SentMailLearned struct {
	Read           int
	Added          int
	AlreadyPresent int
	NoOwnWords     int
	Summary        CorpusSummary
}

const (
	// sentMailLimit is how many of the newest sent mails one press reads.
	sentMailLimit = 200
	// sentMailMinWords drops a mail like "Thanks!", which says nothing about
	// how its sender writes. A short, direct mail still counts.
	sentMailMinWords = 3
	// sentMailRefPrefix keys a source on the mail it was read from.
	sentMailRefPrefix = "sent-mail:"
	// sentMailExclusionCleared is the reason ClearCorpus writes. A press after
	// a clear is a new request to learn, so it reads those mails again. A mail
	// the owner removed or excluded one by one stays out.
	sentMailExclusionCleared = "corpus_cleared"
)

// WithSentMail returns a store that can read the owner's own sent mail.
func (s *VoiceStore) WithSentMail(read SentMailReader) *VoiceStore {
	copied := *s
	copied.sentMail = read
	return &copied
}

// WithVoiceSentMail rebinds the handler set's voice store with the reader of
// the signed-in user's own sent mail.
func (h Handlers) WithVoiceSentMail(read SentMailReader) Handlers {
	h.voice = h.voice.WithSentMail(read)
	return h
}

// LearnFromSentMail files the owner's own words from their newest sent mails
// as email sources of their voice. Quoted replies, forwards and signatures are
// cut first. A mail that is already a source is left as it is. So a press
// never brings back a mail the owner removed, nor one they excluded. All
// sources of one press are written in one transaction. No model is called.
func (s *VoiceStore) LearnFromSentMail(ctx context.Context, profileID ids.UUID) (SentMailLearned, error) {
	if err := auth.Require(ctx, "voice_profile", principal.ActionUpdate); err != nil {
		return SentMailLearned{}, err
	}
	// The owner check runs before any mail is read: a caller who may not
	// write this profile learns nothing about anybody's mailbox.
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		profile, err := s.visibleProfile(ctx, tx, profileID)
		if err != nil {
			return err
		}
		return ownerOnly(ctx, profile)
	}); err != nil {
		return SentMailLearned{}, err
	}
	var learned SentMailLearned
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		learned = SentMailLearned{}
		var mails []SentMail
		if s.sentMail != nil {
			var err error
			if mails, err = s.sentMail(ctx, tx, sentMailLimit); err != nil {
				return err
			}
		}
		learned.Read = len(mails)
		prepared, err := s.prepareSentMail(ctx, mails, &learned)
		if err != nil {
			return err
		}
		return s.fileSentMail(ctx, tx, profileID, prepared, &learned)
	})
	if err != nil {
		return SentMailLearned{}, err
	}
	return learned, nil
}

// prepareSentMail runs each mail's own words through the ingest pipeline, the
// same one an uploaded source takes. A mail the pipeline refuses, for example
// one whose every line quotes a colleague, has no own words to learn from.
func (s *VoiceStore) prepareSentMail(ctx context.Context, mails []SentMail, learned *SentMailLearned) ([]preparedSource, error) {
	prepared := make([]preparedSource, 0, len(mails))
	for _, mail := range mails {
		own := textlang.OwnWords(mail.Body)
		if WordCount(own) < sentMailMinWords {
			learned.NoOwnWords++
			continue
		}
		sentAt := mail.SentAt
		source, err := prepareSource(ctx, IngestSourceInput{
			Kind:        voiceSourceKindEmail,
			SourceLabel: sentMailLabel(mail),
			SourceRef:   sentMailRefPrefix + mail.Ref,
			Format:      corpusFormatTxt,
			Content:     own,
			OccurredAt:  &sentAt,
		}, s.knownSpeakers)
		if _, refused := errors.AsType[*CorpusIngestError](err); refused {
			learned.NoOwnWords++
			continue
		}
		if err != nil {
			return nil, err
		}
		if source.Words < sentMailMinWords {
			learned.NoOwnWords++
			continue
		}
		source.Origin = voiceOriginCapture
		prepared = append(prepared, source)
	}
	return prepared, nil
}

// sentMailLabel names a source in the owner's list by its subject and day.
func sentMailLabel(mail SentMail) string {
	day := mail.SentAt.UTC().Format(time.DateOnly)
	subject := textcut.Runes(mail.Subject, maxSourceLabelChars-len(day)-3)
	if subject == "" {
		return day
	}
	return subject + " · " + day
}

// fileSentMail writes the prepared sources under the profile's corpus lock,
// skipping every mail that already has a row the owner has not cleared.
func (s *VoiceStore) fileSentMail(ctx context.Context, tx pgx.Tx, profileID ids.UUID, prepared []preparedSource, learned *SentMailLearned) error {
	if err := lockProfileForCorpusWrite(ctx, tx, profileID); err != nil {
		return err
	}
	profile, err := s.visibleProfile(ctx, tx, profileID)
	if err != nil {
		return err
	}
	if err := ownerOnly(ctx, profile); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	kept, err := keptSentMailRefs(ctx, tx, profileID)
	if err != nil {
		return err
	}
	for _, source := range prepared {
		if kept[source.SourceRef] {
			learned.AlreadyPresent++
			continue
		}
		row, prior, err := s.persistPreparedSource(ctx, tx, profileID, source, actor.ID)
		if err != nil {
			return err
		}
		if _, err := s.recordSourceIngest(ctx, tx, profile, row, prior); err != nil {
			return err
		}
		learned.Added++
	}
	learned.Summary, err = corpusSummary(ctx, tx, profileID)
	return err
}

// keptSentMailRefs are the sent-mail sources a press leaves alone: live,
// excluded or removed by the owner. That is every row ClearCorpus did not empty.
func keptSentMailRefs(ctx context.Context, tx pgx.Tx, profileID ids.UUID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT source_ref FROM voice_corpus_source
		 WHERE voice_profile_id = $1 AND starts_with(source_ref, $2)
		   AND coalesce(exclusion_reason, '') <> $3`,
		profileID, sentMailRefPrefix, sentMailExclusionCleared)
	if err != nil {
		return nil, err
	}
	kept := map[string]bool{}
	var ref string
	_, err = pgx.ForEachRow(rows, []any{&ref}, func() error {
		kept[ref] = true
		return nil
	})
	return kept, err
}
