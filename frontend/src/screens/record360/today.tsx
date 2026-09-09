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
  Badge,
  Button,
  EmptyState,
  Skeleton,
} from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
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
 *
 * Indigo in every state, because the pane's answer is a machine's whichever
 * one it is giving: the moves are what the agent found, and "nothing needs
 * you" is its reading too.
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
  // The band under the rows: what the day counts down to, and what the day was
  // read from. It is a band and not a line in the head — a caller hands in a
  // commitment badge, a truncation count and the read's own provenance, and
  // three blocks wedged beside the title left nothing whole.
  footer?: ReactNode;
  // A sentence about what the rows could NOT be assembled from — a withheld
  // section — drawn under them whatever else is there. A brief assembled from
  // some of its sources is not the same brief, and the reader is the only one
  // who can judge whether the missing one mattered.
  notice?: ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  // Counted rather than tested for truthiness: a caller hands in arrays, and
  // an empty array is truthy.
  const rows = Children.toArray(children);
  // Panel's own head, rather than one this pane stacks for itself: the title,
  // the disclosure beside it, and the way to the list at the far end. A
  // tone="ai" head is already built for a title that takes two lines
  // (panel.css), which is what a pane drawn at phone width needs — and the
  // head this pane used to draw could not have it, because the title, the
  // badge and the read's three-part line all competed for one row, and a
  // `.badge` is a shrinkable flex item like any other: "AI-assisted" came out
  // as "AI-" over "assisted".
  //
  // Stated ONCE for all three states rather than per branch: a skeleton or an
  // error under a different head is a reader unable to tell whether they are
  // looking at the same pane.
  return (
    <Panel
      tone="ai"
      className="co-reading-today"
      title={t("today.title")}
      titleAction={
        <div className="co-reading-today-actions">
          {/* The rows under this head are the agent's reading of the record —
              what it found and what it prepared — so the claim is read before
              any of them. */}
          <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
          {onOpenTasks && (
            <button type="button" className="link-button" onClick={onOpenTasks}>
              {t("co.suggest.viewTasks")}
            </button>
          )}
        </div>
      }
      footer={footer}
    >
      {state === "loading" && (
        <PanelBody>
          <Skeleton width="100%" height={64} />
        </PanelBody>
      )}
      {state === "failed" && (
        <PanelBody>
          <EmptyState>{t("today.failed")}</EmptyState>
        </PanelBody>
      )}
      {state === "ready" &&
        (rows.length === 0 ? (
          // Not "nothing to do": the brief read everything it can read and
          // found nothing that needs a person today. That is a real answer and
          // it is different from the record being empty.
          <PanelBody>
            <EmptyState>{t("today.quiet")}</EmptyState>
          </PanelBody>
        ) : (
          rows
        ))}
      {/* Under whatever the read produced, and only once it has settled: a
          withheld-sources line over a skeleton describes a reading that does
          not exist yet. */}
      {state === "ready" && notice}
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
    <p className="today-withheld t-caption">
      {t("today.withheld", { sections: sections.join(", ") })}
    </p>
  );
}

/**
 * FoundMove is the move the agent is asking for: whose ask it is and when it
 * was read, the ask at the row's loudest weight, the reason under it — which
 * is the part a rep judges — and, opposite all of it, what a reader can do
 * about it.
 *
 * Under the reason sit the records it was read from, each a chip that opens
 * the record's own words on hover. They are in the row rather than behind a
 * second hover, because a reader questioning the advice first has to find the
 * working — and a popover that listed the same chips the reader would then
 * have to open said "record" and, opened, said "record" again.
 */
export function FoundMove({
  when,
  title,
  why,
  basis,
  action,
  defer,
  suggested = true,
}: Readonly<{
  // Whether the agent is ASKING for this move. False on the answer that
  // nothing needs doing today: nobody suggested that, and the byline over it
  // spent the card's one authorship claim on a non-event — it read as a find
  // where the row's whole point is that there was nothing to find.
  suggested?: boolean;
  // When the reading behind the row is dated. Never a deadline the system
  // chose.
  when?: string;
  title: ReactNode;
  // The reason, which is the part a rep judges. Absent when the ask IS the
  // reason — a move written as one sentence has nothing to put under itself.
  why?: ReactNode;
  // The records the reason was read from, listed under it with what they
  // rest on one hover away. With no reason to hang under, the records are
  // listed in the reason's place.
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
      {/* A div, not a span, and the basis under it too. The row's grounds may
          be an EmailEntry — a block row with a subject, a sender and a preview
          — and a block inside a span is invalid HTML the browser silently
          reflows. CSS can make it LOOK right; it cannot make the document
          tree the one the styles were written against. */}
      <div className="co-move-body">
        {suggested && (
          <span className="co-move-by">
            <Sparkles aria-hidden="true" className="co-move-spark" />
            {t("co.suggest.byline")}
            {when && <span className="t-mono co-move-when">{when}</span>}
          </span>
        )}
        <span className="co-move-ask">{title}</span>
        {why && <span className="co-move-reason t-sub">{why}</span>}
        {basis && (
          <div className="co-move-basis">
            <span className="co-move-basis-head t-eyebrow">
              {t("co.suggest.basedOn")}
            </span>
            {basis}
          </div>
        )}
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
      </div>
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
        {meta && <span className="co-todo-meta t-caption">{meta}</span>}
      </span>
      {due && (
        <span
          className={[
            "co-todo-due",
            "t-caption",
            due.tone ? `co-todo-due-${due.tone}` : "",
          ]
            .filter(Boolean)
            .join(" ")}
        >
          {due.label}
        </span>
      )}
      {verb && (
        <Button
          small
          // Tinted, not filled: three filled buttons down a column outshout
          // the one move above them that the pane is actually recommending,
          // and `aiQuiet` is that volume for an agent's verb among equals.
          variant={verb.byMargince ? "aiQuiet" : "ghost"}
          onClick={verb.onAct}
        >
          {verb.byMargince && <Sparkles aria-hidden="true" />}
          {verb.label}
        </Button>
      )}
    </PanelRow>
  );
}
