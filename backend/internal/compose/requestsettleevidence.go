// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settleEvidence is what the model judges a request on: the request and the
// messages of its thread that followed, and our answers from outside the
// thread, each once, in time order and cut to settleThreadMessages.
//
// The window keeps the request, the newest answer and then the NEWEST of the
// rest. The verdict is recorded through the newest answer, and the prompt's own
// rules turn on what came after our reply, a re-ask above all, so it is the
// middle of a long exchange that is left out. A request whose newest answer can
// no longer be read under these filters is not judged (nil).
func settleEvidence(ctx context.Context, tx pgx.Tx, request activities.RepliedRequest,
	asOf time.Time,
) ([]threadMessage, error) {
	messages, err := conversationRows(ctx, tx, request.RequestID, request.CounterpartyEmail, asOf,
		nil, settleEvidenceFetch)
	// The request leads the evidence or nothing is judged: narrowed since the
	// candidate read, it is gone from the window, and the first reply would be
	// read as the ask.
	if err != nil || len(messages) == 0 || messages[0].ID != request.RequestID {
		return nil, err
	}
	answers, err := activities.OffThreadAnswersTx(ctx, tx, request.RequestID, asOf,
		extractBodyLimit, settleEvidenceFetch)
	if err != nil {
		return nil, fmt.Errorf("request settle: reading the answers off the thread: %w", err)
	}
	seen := make(map[ids.UUID]bool, len(messages)+len(answers))
	for _, message := range messages {
		seen[message.ID] = true
	}
	for _, answer := range answers {
		if seen[answer.ID] {
			continue
		}
		seen[answer.ID] = true
		message := threadMessage{
			ID: answer.ID, Direction: string(crmcontracts.ActivityDirectionOutbound), Subject: answer.Subject,
			Body: answer.Body, At: answer.At, OffThread: answer.Kind,
		}
		if answer.SameThread {
			message.OffThread = ""
		}
		messages = append(messages, message)
	}
	if !seen[request.NewestAnswerID] {
		newest, err := conversationRows(ctx, tx, request.RequestID, request.CounterpartyEmail, asOf,
			&request.NewestAnswerID, 1)
		if err != nil || len(newest) == 0 {
			return nil, err
		}
		messages = append(messages, newest...)
	}
	inTimeOrder(messages)
	return windowKeeping(messages, request.NewestAnswerID, settleThreadMessages), nil
}

// settleEvidenceFetch is how much of each evidence read the window chooses
// from: enough that its newest messages are among them on any exchange a
// window could hold.
const settleEvidenceFetch = 4 * settleThreadMessages

// inTimeOrder sorts messages by (time, id) after the request, which stays
// first: the order the thread read itself keeps, so a tie reads the same way
// on every pass.
func inTimeOrder(messages []threadMessage) {
	if len(messages) < 2 {
		return
	}
	rest := messages[1:]
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].At.Equal(rest[j].At) {
			return rest[i].ID.String() < rest[j].ID.String()
		}
		return rest[i].At.Before(rest[j].At)
	})
}

// windowKeeping cuts messages, request first and the rest in time order, to
// size: the request, the message keep names, and the newest of the others,
// still in time order.
func windowKeeping(messages []threadMessage, keep ids.UUID, size int) []threadMessage {
	if len(messages) <= size {
		return messages
	}
	kept := map[int]bool{0: true}
	room := size - 1
	for i := 1; i < len(messages); i++ {
		if messages[i].ID == keep {
			kept[i] = true
			room--
		}
	}
	for i := len(messages) - 1; i > 0 && room > 0; i-- {
		if !kept[i] {
			kept[i] = true
			room--
		}
	}
	out := make([]threadMessage, 0, size)
	for i, message := range messages {
		if kept[i] {
			out = append(out, message)
		}
	}
	return out
}
