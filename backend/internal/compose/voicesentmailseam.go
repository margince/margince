// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which sent mail may teach a voice? The ai module files the owner's words.
// The answers it needs live in modules it may not import. Activities owns the
// stored mail and its own-sent-mail stamp. Capture owns the rule that tells a
// seat's own address from a mailbox it shares. It also owns the subject
// markers a sender writes to mark a mail confidential.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// voiceSentMail reads the signed-in user's own sent mail for their voice. A
// mail stored from a mailbox another seat also holds is a shared mailbox's,
// not the user's. A mail whose subject marks it confidential is left out even
// when someone has since opened it to the workspace.
func voiceSentMail(pool *pgxpool.Pool) ai.SentMailReader {
	store := activities.NewStore(InstallationDB(pool))
	return func(ctx context.Context, tx pgx.Tx, limit int) ([]ai.SentMail, error) {
		sent, err := store.OwnSentMailTx(ctx, tx, limit)
		if err != nil {
			return nil, err
		}
		mails := make([]ai.SentMail, 0, len(sent))
		for _, mail := range sent {
			if capture.ExplicitlyConfidential(mail.Subject) {
				continue
			}
			own, err := capture.MailboxIsOwnAloneTx(ctx, tx, mail.CapturedBy)
			if err != nil {
				return nil, err
			}
			if !own {
				continue
			}
			mails = append(mails, ai.SentMail{
				Ref: mail.ActivityID.String(), Subject: mail.Subject, Body: mail.Body, SentAt: mail.SentAt,
			})
		}
		return mails, nil
	}
}

// recheckVoiceCopiesTx empties the voice samples read from a mail that may no
// longer feed a voice. Such a mail was archived, restricted, kept from the
// workspace, or marked confidential since it was learned.
func recheckVoiceCopiesTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID, now time.Time) error {
	eligible, subject, err := activities.VoiceEligibleTx(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if eligible && !capture.ExplicitlyConfidential(subject) {
		return nil
	}
	return ai.EraseSamplesCopiedFromTx(ctx, tx, []ids.UUID{activityID}, now)
}
