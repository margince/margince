// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const channelReconfirmationPrefix = "channel-reconfirmation:"

func channelReconfirmation(pool *pgxpool.Pool) chatsync.ReconfirmationWriter {
	dispatcher := NewCommitmentDispatcher(pool, NewApprovalsService(InstallationDB(pool)))
	return func(ctx context.Context, tx pgx.Tx, note ids.UUID, record ids.Ref, question chatsummary.Reconfirmation) error {
		current, err := activities.ChatReconfirmationCurrentTx(ctx, tx, note, question.Quote)
		if err != nil || !current {
			return err
		}
		owner, _, err := recordowner.ReadTx(ctx, tx, record.Type, record.ID)
		if err != nil || owner == nil {
			return err
		}
		state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](*owner))
		if err != nil || state == activities.SeatCannotHoldWork {
			return err
		}
		digest := sha256.Sum256([]byte(note.String() + "\x00" + question.Quote))
		locator := channelReconfirmationPrefix + hex.EncodeToString(digest[:])
		written, err := dispatcher.tasks.CommitmentTaskWritten(ctx, tx, locator)
		if err != nil || written {
			return err
		}
		links := []activities.ActivityLinkInput{{EntityType: record.Type, EntityID: record.ID}}
		commitment := Commitment{SourceActivityID: note, Locator: locator, Evidence: approvals.Evidence{SourceType: string(recordTypeActivity), SourceID: note, Snippet: question.Quote}}
		language := textlang.Detect(question.Question)
		metadata := &systemtext.Message{Version: 1, Subject: systemtext.Prose(systemtext.Generated, language), Body: systemtext.Prose(systemtext.Generated, language)}
		proposal := CommitmentTaskProposal{SystemText: metadata, ReconfirmSource: true, SourceActivityID: note, Summary: question.Question, SeatID: owner, Quote: question.Quote, Locator: locator, Links: links, Body: question.Question}
		_, _, err = dispatcher.stageCommitmentProposal(ctx, tx, commitment, proposal, ids.Nil, *owner, nil)
		return err
	}
}
