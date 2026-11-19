// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// threading is the RFC 5322 conversation chain one outbound message
// carries, plus the key the timeline files it under.
type threading struct {
	inReplyTo  string
	references []string
	threadKey  string
	// splitFrom is the stored key of the conversation a reply on a new topic
	// leaves, empty when the reply stays in it. answers is then the anchor.
	splitFrom string
	answers   ids.UUID
}

// anchorThreading derives the chain a reply carries from its anchor, inside the
// staging transaction. The row-scope check runs again here, because the staged
// delivery names this record.
//
// The chain is rooted at the anchor's thread_key. The recipient's client sets
// References to ours plus our Message-ID, and capture keys a message by the
// first References entry. So the recipient's answer lands in this conversation.
// An activity stores no References, so the chain holds the root and the parent.
//
// A reply on a new topic starts its own conversation, by the rule capture
// applies to mailbox copies (mailsubject.TopicsDifferSQL). It carries no
// In-Reply-To and no References. Every provider, mail client and echo then
// derives its own Message-ID as the key, as this row stores it. A conversation
// a seat has not judged yet does not split. Its pending question names the old
// message, so the reply waits in it with the rest.
func anchorThreading(ctx context.Context, tx pgx.Tx, id ids.ActivityID, messageID, subject string) (threading, error) {
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return threading{}, err
	}
	// Live rows only, as the origin read before this transaction: a reply must
	// not thread onto a conversation archived in between.
	var kind, parent, stored string
	var newTopic bool
	err := tx.QueryRow(ctx,
		`SELECT kind, coalesce(source_id, ''), coalesce(thread_key, ''), `+mailsubject.TopicsDifferSQL("a.subject", "$2")+`
		        AND NOT EXISTS (SELECT 1 FROM capture_thread_verdict v WHERE v.thread_key = a.thread_key AND v.status = $3)
		   FROM activity a
		  WHERE id = $1 AND restricted_at IS NULL AND archived_at IS NULL`,
		id, subject, threadverdict.Pending).Scan(&kind, &parent, &stored, &newTopic)
	if errors.Is(err, pgx.ErrNoRows) {
		return threading{}, apperrors.ErrNotFound
	}
	if err != nil {
		return threading{}, err
	}
	// Only a mail activity's ids are RFC822 message identities. A meeting or a
	// note names no message, so a reply to one starts a conversation.
	parent, root := messageIdentity(kind, parent), messageIdentity(kind, stored)
	if root != "" && newTopic {
		return threading{threadKey: messageID, splitFrom: stored, answers: id.UUID}, nil
	}

	chain := threading{inReplyTo: parent, threadKey: root}
	if root != "" && root != parent {
		chain.references = append(chain.references, root)
	}
	if parent != "" {
		chain.references = append(chain.references, parent)
	}
	if chain.threadKey == "" {
		// An anchor with no stored key starts a conversation rooted at this
		// send's own identity, as capture keys a root message. Never the
		// anchor's source_id: the REST door lets a caller type it. A typed key
		// would let consent find reply evidence in a stranger's thread.
		chain.threadKey = messageID
	}
	return chain, nil
}
