// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag scout's model path. A pass reads the captured items not yet read
// under the workspace's tag vocabulary and asks the model about them in batches.
// Each answered batch commits with its read stamp. No transaction is open while
// the model is asked.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/tagsuggest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tagSuggestItemCap bounds the items one pass reads per workspace; the next
// pass reaches the rest, newest first.
const tagSuggestItemCap = 200

// TagSuggester runs the model path for one workspace at a time.
type TagSuggester struct {
	pool  *pgxpool.Pool
	brain completer
	log   *slog.Logger
}

// NewTagSuggester builds the model path over the pool and the tag_suggest lane.
func NewTagSuggester(pool *pgxpool.Pool, brain completer, log *slog.Logger) *TagSuggester {
	return &TagSuggester{pool: pool, brain: brain, log: log}
}

// tagSuggestBacklog is what a pass reads before it asks the model anything.
type tagSuggestBacklog struct {
	tags   []collections.SuggestibleTag
	digest string
	items  []tagsuggest.Item
}

// RunWorkspace runs one pass in the workspace bound in ctx, as the scout's
// system principal. A budget stop or a blocked provider ends it cleanly with
// what is committed; with only the offline stand-in bound, the phrase pass runs.
func (s *TagSuggester) RunWorkspace(ctx context.Context, now time.Time) (scoutTally, error) {
	var pass scoutTally
	if err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		pass.Superseded, err = collections.SupersedeStaleTagSuggestionsTx(ctx, tx)
		return err
	}); err != nil {
		return pass, err
	}
	backlog, err := s.readBacklog(ctx, now)
	if err != nil {
		return pass, err
	}
	calls := newSweepCalls(tagSuggestItemCap, tagSuggestBatchSize)
	for batch := range slices.Chunk(backlog.items, tagSuggestBatchSize) {
		if !calls.remain() {
			break
		}
		considered, raised, err := s.judgeBatch(ctx, backlog, batch, calls)
		pass.Considered += considered
		pass.Raised += raised
		if errors.Is(err, ai.ErrUnconfiguredModel) {
			return s.phrasePass(ctx, now, pass)
		}
		if sweepPaused(err) {
			s.log.InfoContext(ctx, "tag suggest: stopping the pass", "raised", pass.Raised, "reason", err)
			return pass, nil
		}
		if err != nil {
			return pass, err
		}
	}
	return pass, nil
}

// phrasePass runs the phrase scout in its own transaction, adding to so far.
func (s *TagSuggester) phrasePass(ctx context.Context, now time.Time, soFar scoutTally) (scoutTally, error) {
	var phrase TagScoutPass
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		phrase, err = RunTagScout(ctx, tx, now)
		return err
	})
	return scoutTally{
		Superseded: soFar.Superseded + phrase.Superseded,
		Considered: soFar.Considered + phrase.Considered,
		Raised:     soFar.Raised + phrase.Raised,
	}, err
}

// readBacklog reads the vocabulary and the unread items in one short
// transaction. No suggestible tag means nothing to read.
func (s *TagSuggester) readBacklog(ctx context.Context, now time.Time) (tagSuggestBacklog, error) {
	var backlog tagSuggestBacklog
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		tags, err := collections.SuggestibleTagsTx(ctx, tx)
		if err != nil || len(tags) == 0 {
			return err
		}
		backlog.tags, backlog.digest = tags, tagsuggest.VocabularyDigest(tags)
		// Filed under a contact or company and unread under the digest, newest first.
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.kind, coalesce(a.subject, ''), left(coalesce(a.body, ''), $4), a.occurred_at
			  FROM activity a
			 WHERE `+tagScoutCitable("a")+`
			   AND a.occurred_at > $1 AND a.occurred_at <= $2
			   AND EXISTS (SELECT 1 FROM activity_link l
			                WHERE l.activity_id = a.id AND l.entity_type IN ('contact', 'company'))
			   AND `+collections.TagInterestUnreadClause("a.id", "$3")+`
			 ORDER BY a.occurred_at DESC, a.id
			 LIMIT $5`,
			now.Add(-tagScoutWindow), now, backlog.digest, tagSuggestBodyLimit, tagSuggestItemCap)
		if err != nil {
			return fmt.Errorf("tag suggest: reading the backlog: %w", err)
		}
		backlog.items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (tagsuggest.Item, error) {
			var item tagsuggest.Item
			err := row.Scan(&item.ID, &item.Kind, &item.Subject, &item.Body, &item.OccurredAt)
			return item, err
		})
		if err != nil {
			return fmt.Errorf("tag suggest: scanning the backlog: %w", err)
		}
		return nil
	})
	return backlog, err
}

// judgeBatch asks about one batch with one call and commits the answer. A batch
// the models decline is asked item by item, so one item they will not read
// does not hold back the rest.
func (s *TagSuggester) judgeBatch(ctx context.Context, backlog tagSuggestBacklog, batch []tagsuggest.Item, calls *sweepCalls) (considered, raised int, err error) {
	if !calls.take() {
		return 0, 0, nil
	}
	answers, err := s.ask(ctx, backlog.tags, batch)
	if declinedByTheModels(err) {
		return s.judgeEach(ctx, backlog, batch, calls)
	}
	if err != nil {
		return 0, 0, err
	}
	return s.commitBatch(ctx, backlog.digest, batch, answers)
}

// judgeEach asks about each item in its own call. An item every rung declines
// is stamped read with no tag, so it is not asked about again under this
// vocabulary.
func (s *TagSuggester) judgeEach(ctx context.Context, backlog tagSuggestBacklog, batch []tagsuggest.Item, calls *sweepCalls) (considered, raised int, err error) {
	for _, item := range batch {
		if !calls.take() {
			return considered, raised, nil
		}
		one := []tagsuggest.Item{item}
		answers, err := s.ask(ctx, backlog.tags, one)
		if declinedByTheModels(err) {
			s.log.WarnContext(ctx, "tag suggest: the models declined one item, which is read with no tag",
				"activity_id", item.ID, "err", err)
			answers, err = nil, nil
		}
		if err != nil {
			return considered, raised, err
		}
		c, r, err := s.commitBatch(ctx, backlog.digest, one, answers)
		considered, raised = considered+c, raised+r
		if err != nil {
			return considered, raised, err
		}
	}
	return considered, raised, nil
}

// ask makes one structured call about items.
func (s *TagSuggester) ask(ctx context.Context, tags []collections.SuggestibleTag, items []tagsuggest.Item) ([]tagSuggestAnswer, error) {
	resp, err := ai.Ask(ctx, s.brain, tagSuggestRequest(tags, items), tagSuggestShapeValid(tags, items))
	if err != nil {
		return nil, err
	}
	answers, err := parseTagSuggestReply(resp.Text, tags, items)
	if err != nil {
		return nil, fmt.Errorf("tag suggest: %w", err)
	}
	return answers, nil
}

// commitBatch records the suggestions one answered batch supports and stamps every
// item in it read, in one transaction.
func (s *TagSuggester) commitBatch(ctx context.Context, digest string, batch []tagsuggest.Item, answers []tagSuggestAnswer) (considered, raised int, err error) {
	activities, tags, err := tagSuggestPairs(answers)
	if err != nil {
		return 0, 0, err
	}
	read := make([]ids.UUID, len(batch))
	for i, item := range batch {
		read[i] = item.ID
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if len(activities) > 0 {
			var err error
			// Links and state are read again here, so an item unlinked or
			// restricted while the model was reading it is never cited.
			considered, raised, err = raiseTagSuggestions(ctx, tx, tagScoutSuggestionCap, `
				WITH ev AS (
				  SELECT DISTINCT p.tag_id, l.entity_type, coalesce(l.contact_id, l.company_id) AS record,
				         a.id AS activity_id, a.occurred_at
				    FROM unnest($1::uuid[], $2::uuid[]) AS p(activity_id, tag_id)
				    JOIN activity a ON a.id = p.activity_id
				    JOIN activity_link l ON l.activity_id = a.id AND l.entity_type IN ('contact', 'company')
				   WHERE `+tagScoutCitable("a")+`
				), `+tagScoutRankedSQL("$3"), activities, tags, tagScoutItemCap)
			if err != nil {
				return err
			}
		}
		return collections.MarkTagInterestReadTx(ctx, tx, digest, read)
	})
	return considered, raised, err
}

// tagSuggestPairs flattens validated answers into parallel activity and tag
// id arrays.
func tagSuggestPairs(answers []tagSuggestAnswer) (activities, tags []ids.UUID, err error) {
	for _, answer := range answers {
		activity, err := ids.Parse(answer.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("tag suggest: item id %q: %w", clampToken(answer.ID), err)
		}
		for _, tag := range answer.Tags {
			tagID, err := ids.Parse(tag)
			if err != nil {
				return nil, nil, fmt.Errorf("tag suggest: tag id %q: %w", clampToken(tag), err)
			}
			activities, tags = append(activities, activity), append(tags, tagID)
		}
	}
	return activities, tags, nil
}
