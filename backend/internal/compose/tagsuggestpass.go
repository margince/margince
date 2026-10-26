// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// No transaction is open while the model is asked.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/tagsuggest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// tagSuggestItemCap bounds the items one pass reads per workspace; the next
// pass reaches the rest, newest first.
const tagSuggestItemCap = 200

func tagSuggestBodySQL(a string) string {
	return `left(coalesce(` + a + `.body, ''), ` + strconv.Itoa(tagSuggestBodyLimit) + `)`
}

// tagSuggestContentDigestSQL hashes the subject and body the prompt reads, so
// an edit to either asks the model again.
func tagSuggestContentDigestSQL(a string) string {
	return `md5(coalesce(` + a + `.subject, '') || chr(10) || ` + tagSuggestBodySQL(a) + `)`
}

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

// tagSuggestReadersSQL keys an item by what auth.ActivityContentClause reads of
// it: its links and the seats its attendance arm admits. A task's arm also reads
// its source, but no task is citable. A limited audience is NULL.
func tagSuggestReadersSQL(a string) string {
	return `CASE WHEN ` + a + `.audience = 'workspace' THEN
	  coalesce((SELECT string_agg(DISTINCT link, ',' ORDER BY link) FROM (
	    SELECT l.entity_type || ':' || coalesce(l.contact_id, l.company_id, l.deal_id, l.lead_id, l.project_id)::text
	      FROM activity_link l WHERE l.activity_id = ` + a + `.id) AS links(link)), '')
	  || ';' ||
	  coalesce((SELECT string_agg(DISTINCT seat::text, ',' ORDER BY seat::text) FROM (
	    SELECT ci.user_id FROM capture_import ci WHERE ci.activity_id = ` + a + `.id
	    UNION SELECT ap.user_id FROM activity_participant ap
	     WHERE ap.activity_id = ` + a + `.id AND ap.user_id IS NOT NULL AND ap.address IS NOT NULL) AS attendance(seat)), '')
	END`
}

// tagSuggestCandidate is one item to ask about. readers is nil when the item
// is asked alone; items with equal readers have one reader set.
type tagSuggestCandidate struct {
	item          tagsuggest.Item
	contentDigest string
	readers       *string
}

// tagSuggestBacklog is what a pass reads before it asks the model anything.
type tagSuggestBacklog struct {
	tags       []collections.SuggestibleTag
	vocabulary string
	candidates []tagSuggestCandidate
}

// RunWorkspace runs one pass in the workspace bound in ctx, as the scout's
// system principal. A budget stop or a blocked provider ends the asking, and
// the answers stored so far are still raised. With only the offline stand-in
// bound, the phrase pass runs instead.
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
	if err != nil || len(backlog.tags) == 0 {
		return pass, err
	}
	read, err := s.askBacklog(ctx, backlog)
	if errors.Is(err, ai.ErrUnconfiguredModel) {
		return s.phrasePass(ctx, now, pass)
	}
	if err != nil {
		return pass, err
	}
	s.log.InfoContext(ctx, "tag suggest: model read", "unread", len(backlog.candidates), "read", read)
	pass.Considered, pass.Raised, err = s.raiseFromAnswers(ctx, now, backlog.vocabulary)
	return pass, err
}

// askBacklog asks about the backlog in batches until it is read or the call
// budget is spent. It reports how many answers it stored; a pause is a clean
// stop.
func (s *TagSuggester) askBacklog(ctx context.Context, backlog tagSuggestBacklog) (int, error) {
	read := 0
	calls := newSweepCalls(tagSuggestItemCap, tagSuggestBatchSize)
	for _, batch := range tagSuggestBatches(backlog.candidates) {
		if !calls.remain() {
			break
		}
		n, err := s.judgeBatch(ctx, backlog, batch, calls)
		read += n
		if errors.Is(err, ai.ErrUnconfiguredModel) {
			return read, err
		}
		if sweepPaused(err) {
			s.log.InfoContext(ctx, "tag suggest: stopping the asking", "read", read, "reason", err)
			return read, nil
		}
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// tagSuggestBatches batches items that share one reader set. An answer for one
// item can borrow from another, and it shows to that item's readers.
func tagSuggestBatches(candidates []tagSuggestCandidate) [][]tagSuggestCandidate {
	var batches [][]tagSuggestCandidate
	filling := map[string]int{}
	for _, c := range candidates {
		if c.readers == nil {
			batches = append(batches, []tagSuggestCandidate{c})
			continue
		}
		if i, ok := filling[*c.readers]; ok && len(batches[i]) < tagSuggestBatchSize {
			batches[i] = append(batches[i], c)
			continue
		}
		filling[*c.readers] = len(batches)
		batches = append(batches, []tagSuggestCandidate{c})
	}
	return batches
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

// readBacklog reads the vocabulary and the items without a current answer in
// one short transaction. No suggestible tag means nothing to read.
func (s *TagSuggester) readBacklog(ctx context.Context, now time.Time) (tagSuggestBacklog, error) {
	var backlog tagSuggestBacklog
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		tags, err := collections.SuggestibleTagsTx(ctx, tx)
		if err != nil || len(tags) == 0 {
			return err
		}
		backlog.tags, backlog.vocabulary = tags, tagsuggest.VocabularyDigest(tags)
		var args []any
		arg := func(v any) string {
			args = append(args, v)
			return "$" + strconv.Itoa(len(args))
		}
		query := `
			SELECT a.id, a.kind, coalesce(a.subject, ''), ` + tagSuggestBodySQL("a") + `, a.occurred_at,
			       ` + tagSuggestContentDigestSQL("a") + `, ` + tagSuggestReadersSQL("a") + `
			  FROM activity a
			 WHERE ` + tagScoutCitable("a") + `
			   AND ` + arg(now.Add(-tagScoutWindow)) + ` < a.occurred_at -- occurred_at raw: the scout window's lower edge
			   AND ` + occurredday.BegunSQL("a", arg(now)) + `
			   AND EXISTS (SELECT 1 FROM activity_link l
			                WHERE l.activity_id = a.id AND l.entity_type IN ('contact', 'company'))
			   AND ` + collections.TagInterestUnreadClause("a.id", arg(backlog.vocabulary), tagSuggestContentDigestSQL("a")) + `
			   AND ` + collections.TagInterestContactsLiveClause("a.id") + `
			 ORDER BY a.occurred_at DESC, a.id
			 LIMIT ` + arg(tagSuggestItemCap)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("tag suggest: reading the backlog: %w", err)
		}
		backlog.candidates, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (tagSuggestCandidate, error) {
			var c tagSuggestCandidate
			err := row.Scan(&c.item.ID, &c.item.Kind, &c.item.Subject, &c.item.Body, &c.item.OccurredAt,
				&c.contentDigest, &c.readers)
			return c, err
		})
		if err != nil {
			return fmt.Errorf("tag suggest: scanning the backlog: %w", err)
		}
		return nil
	})
	return backlog, err
}

// raiseFromAnswers raises suggestions from every answer in the window read
// under the current vocabulary and the item's current text. Links, state and
// suggestibility are read here, so a pair refused or unlinked earlier is raised
// once it stands.
func (s *TagSuggester) raiseFromAnswers(ctx context.Context, now time.Time, vocabulary string) (considered, raised int, err error) {
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) string {
			args = append(args, v)
			return "$" + strconv.Itoa(len(args))
		}
		query := `
			WITH ev AS (
			  SELECT DISTINCT p.tag_id, l.entity_type, coalesce(l.contact_id, l.company_id) AS record,
			         a.id AS activity_id, a.occurred_at
			    FROM activity a
			    CROSS JOIN LATERAL (` + collections.TagInterestAnswerSQL("a.id", arg(vocabulary), tagSuggestContentDigestSQL("a")) + `) p
			    JOIN activity_link l ON l.activity_id = a.id AND l.entity_type IN ('contact', 'company')
			   WHERE ` + tagScoutCitable("a") + `
			     AND ` + arg(now.Add(-tagScoutWindow)) + ` < a.occurred_at -- occurred_at raw: the scout window's lower edge
			   AND ` + occurredday.BegunSQL("a", arg(now)) + `
			), ` + tagScoutRankedSQL()
		var err error
		considered, raised, err = raiseTagSuggestions(ctx, tx, tagScoutSuggestionCap, query, args...)
		return err
	})
	return considered, raised, err
}

// judgeBatch asks about one batch with one call and stores the answers. A
// batch the models decline is asked item by item, so one item they will not
// read does not hold back the rest. It reports how many items it stored.
func (s *TagSuggester) judgeBatch(ctx context.Context, backlog tagSuggestBacklog, batch []tagSuggestCandidate, calls *sweepCalls) (int, error) {
	if !calls.take() {
		return 0, nil
	}
	answers, err := s.ask(ctx, backlog.tags, batch)
	if declinedByTheModels(err) {
		return s.judgeEach(ctx, backlog, batch, calls)
	}
	if err != nil {
		return 0, err
	}
	return len(batch), s.storeTagSuggestAnswers(ctx, backlog.vocabulary, batch, answers)
}

// judgeEach asks about each item in its own call. An item every rung declines
// is stored with no tag until its text or the vocabulary changes. The owed and
// capture sweeps record a decline alike: asking again each pass buys no answer.
func (s *TagSuggester) judgeEach(ctx context.Context, backlog tagSuggestBacklog, batch []tagSuggestCandidate, calls *sweepCalls) (int, error) {
	read := 0
	for _, c := range batch {
		if !calls.take() {
			return read, nil
		}
		one := []tagSuggestCandidate{c}
		answers, err := s.ask(ctx, backlog.tags, one)
		if declinedByTheModels(err) {
			s.log.WarnContext(ctx, "tag suggest: the models declined one item, which is read with no tag",
				"activity_id", c.item.ID, "err", err)
			answers, err = nil, nil
		}
		if err != nil {
			return read, err
		}
		if err := s.storeTagSuggestAnswers(ctx, backlog.vocabulary, one, answers); err != nil {
			return read, err
		}
		read++
	}
	return read, nil
}

// ask makes one structured call about a batch.
func (s *TagSuggester) ask(ctx context.Context, tags []collections.SuggestibleTag, batch []tagSuggestCandidate) ([]tagSuggestAnswer, error) {
	items := make([]tagsuggest.Item, len(batch))
	for i, c := range batch {
		items[i] = c.item
	}
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

// storeTagSuggestAnswers stores an answer for each item in the batch, with no
// tag where the answers name none. Each carries the digest of the text read.
func (s *TagSuggester) storeTagSuggestAnswers(ctx context.Context, vocabulary string, batch []tagSuggestCandidate, answers []tagSuggestAnswer) error {
	tagsOf := map[string][]string{}
	for _, a := range answers {
		tagsOf[a.ID] = a.Tags
	}
	stored := make([]collections.TagInterestAnswer, len(batch))
	for i, c := range batch {
		stored[i] = collections.TagInterestAnswer{ActivityID: c.item.ID, ContentDigest: c.contentDigest}
		for _, tag := range tagsOf[c.item.ID.String()] {
			tagID, err := ids.Parse(tag)
			if err != nil {
				return fmt.Errorf("tag suggest: tag id %q: %w", clampToken(tag), err)
			}
			stored[i].TagIDs = append(stored[i].TagIDs, tagID)
		}
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return collections.RecordTagInterestAnswersTx(ctx, tx, vocabulary, tagSuggestContentDigestSQL, stored)
	})
}
