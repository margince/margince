// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one thing to do next, drawn as itself.
//
// The queue already answers "what is most important" — it is the first row, and
// it has been since the ranking shipped. What it did not do is SAY so: a reader
// arriving at a list of twenty-five rows has to work out that the top one is
// the answer, and a list is a shape that invites scanning rather than acting.
//
// So the first row is lifted out and given the evidence a rep needs to act
// without opening anything: what it is, why now in money and dates, and the one
// verb the server named. The row stays in the queue below it, because removing
// it would make the rank numbers lie and the counts disagree with the page.

import { Badge } from "../design-system/atoms";
import { Panel } from "../design-system/panel";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { TaskQuickActions, useTaskUpdate } from "./taskactions";
import {
  consequenceText,
  dealFactsText,
  itemTitle,
  moveHref,
  moveOpensComposer,
  reasonText,
  rowHref,
} from "./worklist.copy";
import { type WorklistItem, worklistKey } from "./worklist.queries";

// The focus card, or nothing.
//
// Drawn only on a page the reader can act on. A day whose top row is a
// duplicate-merge suggestion is not a day with a recommended action, and
// promoting one would tell a rep that hygiene is their most important work.
export function FocusCard({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  if (!worthActingOn(item)) {
    return null;
  }
  const href = rowHref(item);
  const move = moveHref(item);
  const facts = dealFactsText(item, t, locale, zone);
  const consequence = consequenceText(item, t);
  // The evidence, in the order the concept asks for it: what, why now, what it
  // costs to do nothing. The comparator is deliberately absent — "182 against
  // 180" belongs behind a disclosure, never in the primary scan path.
  const because = item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null)
    .join(" · ");
  return (
    <Panel title={t("worklist.focus.title")}>
      <div className="worklist-focus">
        <p className="t-h2 worklist-focus-what">
          {href ? (
            <a className="entity-link" href={href}>
              {itemTitle(item, t, locale)}
            </a>
          ) : (
            itemTitle(item, t, locale)
          )}
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        </p>
        {facts && <p className="t-body worklist-focus-facts">{facts}</p>}
        {because && <p className="t-caption worklist-focus-why">{because}</p>}
        {consequence && (
          <p className="t-caption worklist-focus-cost">{consequence}</p>
        )}
        {/* ONE verb. The server named it, and a card offering three would be
            the list again in a bigger box. Everything else this row supports
            stays available on its own row below. */}
        <div className="worklist-focus-verb">
          <FocusVerb item={item} href={href} move={move} />
        </div>
      </div>
    </Panel>
  );
}

// The card's single verb.
//
// Three shapes, and which one is drawn is decided by what the verb can actually
// DO — not by what reads best. A card whose strongest control promises more
// than it performs is worse than one that promises less: the reader believes
// the work is done and moves on.
//
// `complete` on a task is the case that made this a component. The label said
// "Complete it" over a link to the task's record, so pressing it navigated and
// completed nothing. The mutation exists and every other surface already uses
// it, so the card completes the task rather than renaming the promise down.
function FocusVerb({
  item,
  href,
  move,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  move: string | undefined;
}>) {
  const t = useT();
  const update = useTaskUpdate([worklistKey]);
  if (item.source === "task" && item.primary_action === "complete") {
    return (
      <TaskQuickActions
        activityId={item.id}
        dueAt={item.due_at}
        update={update}
      />
    );
  }
  if (move) {
    return (
      <a className="btn btn-primary" href={move}>
        {/* The row makes this distinction and the card used to drop it, so the
            same address was described two ways on one screen. */}
        {t(
          moveOpensComposer(item)
            ? "worklist.verb.draft_reply_now"
            : "worklist.verb.draft_reply",
        )}
      </a>
    );
  }
  if (!href) {
    return null;
  }
  return (
    <a className="btn btn-primary" href={href}>
      {t(`worklist.focus.verb.${item.primary_action ?? "open"}`)}
    </a>
  );
}

// Whether this row is something the reader can act on now.
//
// Exported because the Next-up list asks the same question of the rows after
// the focused one. One rule, two surfaces: a row that list offered and this
// card refused would be the page contradicting itself about one morning.
//
// A recommended action has to be something the rep DOES, and it has to be
// theirs to do now. Review work is neither: it is judgement the queue collects
// so it can be worked through in one pass, and a page headed "do this next"
// over a duplicate-merge suggestion would be telling a rep something false
// about their morning.
export function worthActingOn(item: WorklistItem): boolean {
  if (item.band === "review" || item.primary_action === undefined) {
    return false;
  }
  // `acknowledge` names no record to route to — the same reason WorklistRow's
  // own VERB_DESTINATION table excludes it. Excluded here explicitly, not
  // left to `rowHref` coming back undefined: a notice ever gaining a subject
  // href would otherwise arm this as a link to nowhere the reader clicks
  // expecting "I've seen this".
  if (item.primary_action === "acknowledge") {
    return false;
  }
  // And somewhere for the verb to GO. A row filed under no record — a task
  // nobody linked to anything — has no address at all: rowHref falls through
  // the subject and the source-queue map and finds none. The card would then
  // be a headline with no way to act, occupying the one place a reader looks
  // for their next step.
  return rowHref(item) !== undefined || moveHref(item) !== undefined;
}

// Whether the page should draw a card at all, asked of the whole queue so the
// screen does not have to know the rule.
export function focusOf(
  queue: readonly WorklistItem[],
): WorklistItem | undefined {
  const first = queue[0];
  return first && worthActingOn(first) ? first : undefined;
}
