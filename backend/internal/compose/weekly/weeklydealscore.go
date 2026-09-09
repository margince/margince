// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The deal block's one statement.
//
// Beside the reconstruction it depends on rather than beside the Go that binds
// its arguments: the CTE this composes with — week_end, from weeklyweekend.go —
// is what makes the population counts describe the week rather than today, and
// the two read as one piece of reasoning.

// dealScoreSQL is the deal block's one statement.
//
// One statement rather than ten, because ten counts read at ten moments
// describe ten slightly different weeks. Lifted out of the function so the
// arithmetic beside it stays readable at a glance; the format verbs are bound
// at the single call site below.
const dealScoreSQL = `
		WITH mine AS (
		  SELECT d.id, d.stage_id, d.pipeline_id, d.expected_close_date,
		         d.close_date_provisional, d.archived_at, d.status
		    FROM deal d
		   WHERE d.owner_id = $%[3]d AND (%[8]s)
		     -- Existing at the cutoff is a precondition of being IN the week. The
		     -- rewind below says what a deal LOOKED like then; it cannot say
		     -- whether it was there, so a deal minted after the week closed would
		     -- otherwise be folded into a count of that week's pipeline at its
		     -- birth values.
		     AND d.created_at < $%[2]d),
		%[10]s,
		-- The population counts read the WEEK-END row, never the current one. A
		-- deal closed on Tuesday was open when the week ended, and belongs in the
		-- denominator of the week being judged.
		open_deals AS (
		  SELECT * FROM week_end
		   WHERE status = 'open' AND NOT was_archived AND NOT unreconstructible),
		-- Stage moves inside the window, with both ends' positions resolved.
		-- A move whose two stages sit in different pipelines resolves to NULL
		-- on one side and is counted as neither direction.
		moves AS (
		  SELECT h.deal_id, h.changed_at, h.from_stage_id,
		         fs."position" AS from_pos, ts."position" AS to_pos
		    FROM deal_stage_history h
		    JOIN mine ON mine.id = h.deal_id
		    LEFT JOIN stage fs ON fs.id = h.from_stage_id
		         AND fs.pipeline_id = mine.pipeline_id
		    LEFT JOIN stage ts ON ts.id = h.to_stage_id
		         AND ts.pipeline_id = mine.pipeline_id
		   WHERE h.changed_at >= $%[1]d AND h.changed_at < $%[2]d
		     AND h.to_stage_id IS DISTINCT FROM h.from_stage_id),
		-- How long the deal sat in the stage it LEFT: this move's moment less
		-- the previous move into that stage. A deal whose prior move is outside
		-- the window still resolves, because the lookup is not windowed.
		dwell AS (
		  SELECT m.deal_id,
		         EXTRACT(epoch FROM m.changed_at - (
		           SELECT max(p.changed_at) FROM deal_stage_history p
		            WHERE p.deal_id = m.deal_id AND p.changed_at < m.changed_at
		         )) / 86400 AS days
		    FROM moves m
		   WHERE m.from_stage_id IS NOT NULL),
		-- Category moves, one row per deal: a rep who corrected a typo three
		-- times did not downgrade three times. The direction is the deal's
		-- FIRST and LAST category inside the window, compared once.
		--
		-- No erasure boundary here, unlike the rewind above, and the difference is
		-- what the two read. The rewind folds an image back onto a record a reader
		-- sees; this compares one enum against itself and keeps neither value. A
		-- forecast category is a four-valued CHECK — omitted, pipeline, best_case,
		-- commit — carrying nothing a scrub certifies gone, so stopping at a
		-- tombstone would withhold a direction while disclosing nothing.
		cat AS (
		  SELECT a.entity_id,
		         (array_agg(a.before->>'forecast_category'
		           ORDER BY a.occurred_at, a.id))[1] AS first_cat,
		         (array_agg(a.after->>'forecast_category'
		           ORDER BY a.occurred_at DESC, a.id DESC))[1] AS last_cat
		    FROM audit_log a
		    JOIN mine ON mine.id = a.entity_id
		   WHERE a.entity_type = 'deal'
		     AND a.occurred_at >= $%[1]d AND a.occurred_at < $%[2]d
		     AND a.before->>'forecast_category' IS DISTINCT FROM a.after->>'forecast_category'
		   GROUP BY a.entity_id)
		SELECT
		  EXISTS (SELECT 1 FROM open_deals) OR EXISTS (SELECT 1 FROM moves)
		    -- A week whose every deal is behind an erasure still HAS a deal block.
		    -- Without this the block vanishes, which reads as 'this rep had no
		    -- deals' — the one thing that is certainly false. It is present,
		    -- its counts are zero, and the shortfall beside them says why.
		    OR EXISTS (SELECT 1 FROM week_end WHERE unreconstructible),
		  (SELECT count(*) FROM moves WHERE from_pos IS NOT NULL AND to_pos > from_pos),
		  (SELECT count(*) FROM moves WHERE from_pos IS NOT NULL AND to_pos < from_pos),
		  (SELECT round(percentile_cont(0.5) WITHIN GROUP (ORDER BY days))::int
		     FROM dwell WHERE days IS NOT NULL),
		  (SELECT count(*) FROM open_deals o
		    WHERE EXISTS (
		      SELECT 1 FROM activity_link tl
		        JOIN activity task ON task.id = tl.activity_id
		       WHERE tl.deal_id = o.id AND task.kind = 'task'
		         AND task.archived_at IS NULL
		         -- The task as the week CLOSED, not as it stands now. A task
		         -- ticked off on Monday was still the deal's open next step on
		         -- Sunday, and reading is_done today erases it from the week that
		         -- earned it. Created-after is excluded for the mirror reason: a
		         -- task written on Monday was not Sunday's next step.
		         AND task.created_at < $%[2]d
		         -- Done BEFORE the cutoff means it was not the deal's open next
		         -- step then. done_at travels with is_done and is CLEARED when a
		         -- task is reopened, so a task finished in-week and reopened after
		         -- carries no stamp: is_done is false and the task counts, which
		         -- is right — it was open at the cutoff either way only if it was
		         -- not finished before it, and a cleared stamp cannot say it was.
		         -- The residual gap is a task finished in-week, reopened after, and
		         -- finished again: the second stamp is post-cutoff, so it counts.
		         AND NOT (task.is_done AND task.done_at IS NOT NULL
		                  AND task.done_at < $%[2]d))),
		  (SELECT count(*) FROM open_deals),
		  (SELECT count(*) FROM open_deals o
		    WHERE (SELECT count(DISTINCT pl.person_id)
		             FROM activity_link dl
		             JOIN activity act ON act.id = dl.activity_id
		             JOIN activity_link pl ON pl.activity_id = dl.activity_id
		            WHERE dl.deal_id = o.id AND pl.person_id IS NOT NULL
		              AND act.archived_at IS NULL
		              -- BOUNDED AT BOTH ENDS. The lower bound is the 30-day
		              -- coverage window; the upper is the cutoff, without which
		              -- two contacts who first spoke to the deal on Monday would
		              -- make the closed week read multi-threaded when it was not.
		              AND act.occurred_at >= $%[4]d
		              AND act.occurred_at < $%[2]d) >= $%[5]d),
		  (SELECT count(*) FROM open_deals o
		    WHERE o.expected_close_date IS NOT NULL
		      AND NOT o.close_date_provisional
		      AND o.expected_close_date >= (timezone($%[9]d, $%[2]d))::date),
		  (SELECT count(*) FROM cat WHERE %[6]s > %[7]s),
		  (SELECT count(*) FROM cat WHERE %[6]s < %[7]s),
		  (SELECT count(*) FROM week_end WHERE unreconstructible)`
