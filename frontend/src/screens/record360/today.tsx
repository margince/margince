// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHAT NEEDS A PERSON TODAY — the one part of a record page that answers
// "what do I do now", and the rows it is made of.
//
// Two kinds of row, at two weights, and the difference is who is asking. A
// FOUND MOVE is the one thing the agent is asking for, argued in a sentence
// with the records it rests on behind it; a TO-DO is a commitment the record
// already carries, which a reader scans. At one weight the two read as five
// equal asks and the one worth doing first stops leading.
//
// The panel and its rows came out of the company page's daily brief. The
// contact page had a card of its own for the moment and the deal page a
// paragraph inside its briefing, and a rep who reads three records met three
// spellings of "here is the move" — so the chrome, the empty answer and the
// two row shapes are one component each, and a record hands in its rows.

import { Sparkles } from "lucide-react";
import { Children, type ReactNode } from "react";
import {
  Avatar,
  Button,
  EmptyState,
  Skeleton,
} from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { useT } from "../../i18n";
import "../company360.css";

/**
 * TodayPanel is the panel: its head, the rows a caller hands in, and the one
 * honest sentence for a day with no rows.
 *
 * `state` is the read the rows come from. Neither the pending read nor the
 * failed one draws rows — and neither may draw the quiet sentence, which is a
 * claim about the record ("nothing needs you") that a read still in flight
 * has no basis for.
 */
export function TodayPanel({
  state = "ready",
  onOpenTasks,
  footer,
  notice,
  children,
}: Readonly<{
  state?: "ready" | "loading" | "failed";
  // Where the head's link leads. Absent for a record with no task list of its
  // own to open.
  onOpenTasks?: () => void;
  // The band under the rows: what the day counts down to.
  footer?: ReactNode;
  // A sentence about what the rows could NOT be assembled from — a withheld
  // section — drawn under them whatever else is there. A brief assembled from
  // some of its sources is not the same brief, and the reader is the only one
  // who can judge whether the missing one mattered.
  notice?: ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  // The subhead rides every state, because a skeleton or an error under no
  // name is a reader unable to tell WHICH reading is missing.
  const head = (
    <PanelBody className="co-360-head">
      {/* The count beside the name, the mock's `h3 small`: "1 overdue" is a
          fact about the list's head, and as a footer band under the rows it
          floated alone at the bottom of the pane. */}
      <span className="co-360-headtext">
        <Eyebrow as="h3">{t("today.title")}</Eyebrow>
        {footer}
      </span>
      {onOpenTasks && (
        <Button small variant="ghost" onClick={onOpenTasks}>
          {t("co.suggest.viewTasks")}
        </Button>
      )}
    </PanelBody>
  );
  if (state === "loading") {
    return (
      <Panel className="co-reading-today">
        {head}
        <PanelBody>
          <Skeleton width="100%" height={64} />
        </PanelBody>
      </Panel>
    );
  }
  if (state === "failed") {
    return (
      <Panel className="co-reading-today">
        {head}
        <PanelBody>
          <EmptyState>{t("today.failed")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }
  // Counted rather than tested for truthiness: a caller hands in arrays, and
  // an empty array is truthy.
  const rows = Children.toArray(children);
  return (
    <Panel className="co-reading-today">
      {head}
      {rows.length === 0 ? (
        // Not "nothing to do": the brief read everything it can read and found
        // nothing that needs a person today. That is a real answer and it is
        // different from the record being empty.
        <PanelBody>
          <EmptyState>{t("today.quiet")}</EmptyState>
        </PanelBody>
      ) : (
        rows
      )}
      {notice}
    </Panel>
  );
}

/**
 * WithheldNotice names the sources the day's work could NOT be assembled from
 * — sections a grant withheld — under whatever rows there are. "Hidden from
 * you", never "none": a brief assembled from some of its sources is not the
 * same brief, and the reader is the only one who can judge whether the missing
 * one mattered. Nothing when nothing was withheld.
 */
export function WithheldNotice({
  sections,
}: Readonly<{ sections: readonly string[] }>) {
  const t = useT();
  if (sections.length === 0) {
    return null;
  }
  return (
    <p className="today-withheld">
      {t("today.withheld", { sections: sections.join(", ") })}
    </p>
  );
}

/**
 * FoundMove is the move the agent found: who found it and when, the ask at
 * the row's loudest weight, the reason under it — which is the part a rep
 * judges — and, opposite all of it, what a reader can do about it.
 *
 * The reason is also the handle on what the advice rests on. Behind it the
 * records are one glance away for the reader who is questioning the advice,
 * and out of the way of the one who is not.
 */
export function FoundMove({
  when,
  title,
  why,
  basis,
  action,
  defer,
}: Readonly<{
  // When the reading behind the row is dated. Never a deadline the system
  // chose.
  when?: string;
  title: ReactNode;
  // The reason, which is the part a rep judges. Absent when the ask IS the
  // reason — a move written as one sentence has nothing to put under itself.
  why?: ReactNode;
  // The records the reason was read from, shown behind it. Absent, the reason
  // is plain text: a dotted rule that opens nothing teaches the reader that
  // the working is never there. With no reason to hang it on, the records
  // are listed in the reason's place.
  basis?: ReactNode;
  // What performing the move means, as the caller's own control. Absent when
  // the record cannot say — a rule that named no action draws nothing rather
  // than a control that does nothing.
  action?: ReactNode;
  // Putting the move off. Not the row's verb and never drawn as one.
  defer?: { onDefer: () => void; pending?: boolean };
}>) {
  const t = useT();
  return (
    <PanelRow className="co-move">
      <span className="co-move-body">
        <span className="co-move-by">
          <Sparkles aria-hidden="true" className="co-move-spark" />
          {t("co.suggest.found")}
          {when && <span className="t-mono co-move-when">{when}</span>}
        </span>
        <span className="co-move-ask">{title}</span>
        {why && basis && (
          <Popover className="co-move-why" onHover label={why}>
            <span className="co-move-basis-head t-eyebrow">
              {t("co.suggest.basedOn")}
            </span>
            {basis}
          </Popover>
        )}
        {why && !basis && <span className="co-move-reason">{why}</span>}
        {!why && basis && <span className="co-move-reason">{basis}</span>}
        {(action || defer) && (
          <span className="co-move-do">
            <span className="co-move-actions">
              {action}
              {defer && (
                <Button
                  small
                  className="co-move-defer"
                  onClick={defer.onDefer}
                  disabled={defer.pending}
                >
                  {t("co.suggest.dismiss")}
                </Button>
              )}
            </span>
          </span>
        )}
      </span>
    </PanelRow>
  );
}

/**
 * TodoRow is one thing already on somebody's list: who it sits with, what it
 * is, when it is due, and the one verb that advances it.
 *
 * The mark is the row's anchor. These rows are scanned rather than read, and a
 * column of initials is what lets a reader find their own before they have
 * read a word — which is also why the verb is an outlined chip rather than a
 * filled button: filled, three of them outshout the single move above that the
 * panel is actually recommending.
 */
export function TodoRow({
  who,
  title,
  meta,
  due,
  verb,
}: Readonly<{
  // Whose list it sits on. Absent when the record cannot say — an unassigned
  // task draws no mark rather than a monogram of nobody.
  who?: string;
  title: ReactNode;
  meta?: ReactNode;
  // When it is owed, coloured only where it is bad news: a late promise is the
  // one thing on the row that may shout.
  due?: { label: string; tone?: "warn" | "danger" };
  // The verb that advances it. `byMargince` marks a verb whose work the agent
  // does — a draft it writes — because the indigo mark means authorship and
  // nothing else.
  verb?: { label: string; onAct: () => void; byMargince?: boolean };
}>) {
  return (
    <PanelRow className="co-todo">
      {who && <Avatar name={who} size="xs" />}
      <span className="co-todo-body">
        <span className="co-todo-title">{title}</span>
        {meta && <span className="co-todo-meta">{meta}</span>}
      </span>
      {due && (
        <span
          className={["co-todo-due", due.tone ? `co-todo-due-${due.tone}` : ""]
            .filter(Boolean)
            .join(" ")}
        >
          {due.label}
        </span>
      )}
      {verb && (
        <Button
          small
          variant="ghost"
          className={verb.byMargince ? "co-todo-verb" : undefined}
          onClick={verb.onAct}
        >
          {verb.byMargince && <Sparkles aria-hidden="true" />}
          {verb.label}
        </Button>
      )}
    </PanelRow>
  );
}
