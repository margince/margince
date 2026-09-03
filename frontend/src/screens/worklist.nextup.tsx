// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The few rows after the focus card, drawn as a finite list.
//
// The focus card says what to do NEXT and the queue says what there is. Between
// them sat nothing: a reader who had done the one thing the card named went back
// to a list of twenty-five rows and had to re-find their place in it. This is
// the short answer to "and then?" — bounded, so it can be finished, which is the
// whole difference between a plan for a morning and a backlog.
//
// It SELECTS NOTHING. The rows are the ones the server already ranked, taken in
// the order it ranked them. Re-ranking here would be a second answer to what
// matters most today, and two clients would then disagree about one morning.
//
// SO THESE FOUR ROWS CAN ALL BE ONE KIND OF WORK, and that is worth saying
// plainly rather than implying otherwise. The queue's anti-monopoly rule is the
// `crowded` flag, and it reaches only two lanes — unanswered customers and
// overdue leads — each at a threshold of eight. Every other producer (tasks,
// bounces, decisions, the health lanes) has no cap at all, and even the two that
// do cannot fire before position nine. This list reads positions two to four. So
// six overdue tasks make a focus card and three rows that are all the same
// sentence, and nothing in the current ranking prevents it.
//
// A quota HERE is still the wrong fix: the client would be answering a question
// the server owns, and the queue below would disagree with the list above it.
// Widening `crowded` to the other lanes is the fix, and it belongs to the
// ranking rather than here — filed as #3673. Until then this list is honest
// about being the top of one order rather than a survey of the day.

import { Panel } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import { rowHref } from "./worklist.copy";
import { nextUpLine } from "./worklist.emailtitle";
import { worthActingOn } from "./worklist.focus";
import type { WorklistItem } from "./worklist.queries";
import "./worklist.css";

// How many rows the list carries.
//
// Small enough to be a plan rather than a page: a reader has to be able to see
// the end of it without scrolling, or it is the queue again in a smaller box.
// The focus card is the first of the day's work, so this is what follows it.
const NEXT_UP = 3;

/**
 * The rows after the focused one, or nothing.
 *
 * Asked of the whole queue so the screen does not have to know the rule, the
 * way `focusOf` is. Returns the rows in the server's own order.
 */
export function nextUpOf(
  queue: readonly WorklistItem[],
  focused: WorklistItem | undefined,
): readonly WorklistItem[] {
  // Everything the focus card would itself have accepted, minus the row it took.
  // Reusing that rule rather than restating it is what keeps the two surfaces
  // agreeing about what a reader can act on: a row this list offered and the
  // card refused would be the page contradicting itself about the same morning.
  //
  // The focused row is excluded by IDENTITY, not by id, and the two differ on a
  // queue that names one record twice — a person with an unanswered message and
  // an overdue task carries the same `id` on both rows, and excluding by id
  // would drop the second along with the first. Identity is sound here because
  // both this and `focusOf` read the same `day.queue` array in one render, so a
  // refetch replaces the focused row and this list's rows together. `selected`
  // in the screen resolves by id for the opposite reason: it is held across
  // renders in state, where an object goes stale.
  return queue
    .filter((item) => item !== focused && worthActingOn(item))
    .slice(0, NEXT_UP);
}

/**
 * The list. Titles and one link each — the evidence lives on the row below and
 * in the card above, and repeating it here would make three surfaces to keep in
 * agreement about one deal.
 */
export function NextUp({
  items,
}: Readonly<{ items: readonly WorklistItem[] }>) {
  const t = useT();
  const { locale } = useLocale();
  if (items.length === 0) {
    return null;
  }
  return (
    <Panel title={t("worklist.nextup.title")}>
      <ol className="worklist-nextup">
        {items.map((item) => {
          const href = rowHref(item);
          return (
            <li
              key={`${item.source}-${item.id}`}
              className="worklist-nextup-row"
            >
              {/* The SUBJECT for a waiting email, not the canonical row: this
                  is a compact one-line list of what comes after the current
                  focus, and a four-line row per entry would defeat the reason
                  it is a list. The row itself is on the queue and on the focus
                  card, both of which have the room for it. */}
              {href ? (
                <a className="entity-link" href={href}>
                  {nextUpLine(item, t, locale)}
                </a>
              ) : (
                nextUpLine(item, t, locale)
              )}
            </li>
          );
        })}
      </ol>
    </Panel>
  );
}
