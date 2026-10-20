// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHAT NEEDS A CONTACT TODAY — the one part of a record page that answers
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
} from "../../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../../design-system/panel";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import "../../companies/company360.css";
// The verb column: `.today-actions` and `.today-verb`, drawn here rather than
// left to whichever page mounts a row that needs them.
import "./record360.css";

/**
 * TodayPanel is the record's next step: a strip holding the first row a caller
 * hands in, the count of the rest, and the one honest sentence for a day with
 * no rows. The rest stand in a card of their own when the caller names one.
 *
 * `state` is the read the rows come from. Neither the pending read nor the
 * failed one draws rows — and neither may draw the quiet sentence, which is a
 * claim about the record ("nothing needs you") that a read still in flight
 * has no basis for.
 */
export function TodayPanel({
  state = "ready",
  onOpenTasks,
  tasksLabel,
  restTitle,
  footer,
  notice,
  complete = true,
  children,
}: Readonly<{
  state?: "ready" | "loading" | "failed";
  // Where the head's link leads. Absent for a record with no task list of its
  // own to open.
  onOpenTasks?: () => void;
  tasksLabel?: string;
  // The strip draws only the first row. A record that lists the rest names
  // the card they stand in; without one they are counted beside the first.
  restTitle?: string;
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
  // False when a source the rows are read from was withheld: an empty list is
  // then no basis for "nothing needs you", so it draws nothing but the notice.
  complete?: boolean;
  children?: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // Counted rather than tested for truthiness: a caller hands in arrays, and
  // an empty array is truthy.
  const [lead, ...rest] = Children.toArray(children);
  const openTasks = onOpenTasks ? (
    <Button variant="link" onClick={onOpenTasks}>
      {tasksLabel ?? t("co.suggest.viewTasks")}
    </Button>
  ) : undefined;
  // The count of what the strip leaves out, as the way to it when there is one.
  const more = plural("today.more", rest.length, {
    count: formatNumber(rest.length, locale),
  });
  return (
    <>
      {/* The next step as one strip across the sheet: its name, the first
          row, and how much more waits behind it. Neutral, because a row the
          agent found says so in its own byline and a to-do is the record's. */}
      <Panel
        className="co-lead record-next"
        title={t("today.title")}
        footer={footer}
      >
        {state === "loading" && (
          <PanelBody>
            <Skeleton width="100%" height={40} />
          </PanelBody>
        )}
        {state === "failed" && (
          <PanelBody>
            <EmptyState>{t("today.failed")}</EmptyState>
          </PanelBody>
        )}
        {state === "ready" && lead}
        {/* Not "nothing to do": the brief read everything it can read and
            found nothing that needs a contact today, which is a real answer
            and different from the record being empty. */}
        {state === "ready" && lead === undefined && complete && (
          <PanelBody>
            <EmptyState>{t("today.quiet")}</EmptyState>
          </PanelBody>
        )}
        {state === "ready" && (rest.length > 0 || openTasks) && (
          <span className="record-next-more">
            {rest.length > 0 && onOpenTasks ? (
              <Button variant="link" onClick={onOpenTasks}>
                {more}
              </Button>
            ) : (
              (rest.length > 0 && more) || openTasks
            )}
          </span>
        )}
        {/* Under whatever the read produced, and only once it has settled. */}
        {state === "ready" && notice}
      </Panel>
      {state === "ready" && restTitle && rest.length > 0 && (
        <Panel title={restTitle} titleAction={openTasks}>
          {rest}
        </Panel>
      )}
    </>
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
 * FoundMove is the move the agent is asking for: whose ask it is, what it was
 * read against and when, the ask at the row's loudest weight, the reason under
 * it — which is the part a rep judges — and, opposite all of it, what a reader
 * can do about it.
 *
 * Under the reason sit the records it was read from, each a chip that opens
 * the record's own words on hover. They are in the row rather than behind a
 * second hover, because a reader questioning the advice first has to find the
 * working — and a popover that listed the same chips the reader would then
 * have to open said "record" and, opened, said "record" again.
 */
export function FoundMove({
  when,
  kicker,
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
  // What the agent read the record AGAINST — the rule's own name. It
  // qualifies the authorship claim rather than the move, so it is a second
  // clause of the byline and never a line over the ask: set above it, a rule's
  // name reads as the move's own title and the move stops being the sentence
  // the row exists to say.
  kicker?: ReactNode;
  title: ReactNode;
  // The reason, which is the part a rep judges. Absent when the ask IS the
  // reason — a move written as one sentence has nothing to put under itself.
  why?: ReactNode;
  // The records the reason was read from, listed under it with what they
  // rest on one hover away. With no reason to hang under, the records are
  // listed in the reason's place.
  basis?: ReactNode;
  // What performing the move means, as the caller's own control — zero or
  // more verbs, each already in the shared `.today-verb` shape (record360.css)
  // rather than pre-wrapped in a column of its own: this row is the one place
  // that owns the column, so a caller handing in its own `.today-actions`
  // nested inside this one laid the defer button beside it in a row instead
  // of under it. Absent when the record cannot say — a rule that named no
  // action draws nothing rather than a control that does nothing.
  action?: ReactNode;
  // Putting the move off. Not the row's verb and never drawn as one, but
  // still one more item in the same column, last.
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
            {kicker && <span className="co-move-kicker">{kicker}</span>}
            {when && <span className="t-num">{when}</span>}
          </span>
        )}
        <span className="co-move-ask">{title}</span>
        {why && <span className="co-move-reason t-sub">{why}</span>}
        {basis && (
          <div className="co-move-basis">
            <span className="co-move-basis-head t-caption">
              {t("co.suggest.basedOn")}
            </span>
            {basis}
          </div>
        )}
        {(action || defer) && (
          <span className="co-move-do">
            <div className="today-actions">
              {action}
              {defer && (
                <span className="today-verb">
                  <Button
                    variant="ghost"
                    className="co-move-defer"
                    onClick={defer.onDefer}
                    disabled={defer.pending}
                  >
                    {t("co.suggest.dismiss")}
                  </Button>
                </span>
              )}
            </div>
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
  action,
}: Readonly<{
  // Whose list it sits on. Absent when the record cannot say — an unassigned
  // task draws no mark rather than a monogram of nobody. `identity` is their
  // record or seat id, which keys the mark.
  who?: Readonly<{ name: string; identity: string }>;
  title: ReactNode;
  meta?: ReactNode;
  // When it is owed, coloured only where it is bad news: a late promise is the
  // one thing on the row that may shout.
  due?: { label: string; tone?: "warning" | "danger" };
  // The verb that advances it. `byMargince` marks a verb whose work the agent
  // does — a draft it writes — because the indigo mark means authorship and
  // nothing else.
  verb?: { label: string; onAct: () => void; byMargince?: boolean };
  // For a shared action component that owns its drawer and pending state.
  action?: ReactNode;
}>) {
  return (
    <PanelRow className="co-todo">
      {who && <Avatar name={who.name} identity={who.identity} />}
      <span className="co-todo-body">
        <span className="co-todo-title">{title}</span>
        {meta && <span className="t-caption">{meta}</span>}
      </span>
      {/* What is owed and the verb that settles it, as ONE item. They are the
          row's own last two children at reading width — the wrapper draws no
          box there — and the pair is what moves under the title at the fold,
          where a date and a button holding their own widths beside it left the
          title a word wide. */}
      {(due || action || verb) && (
        <span className="co-todo-owed">
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
          {action ??
            (verb && (
              <Button
                // Tinted, not filled: three filled buttons down a column
                // outshout the one move above them that the pane is actually
                // recommending, and `aiQuiet` is that volume for an agent's
                // verb among equals.
                variant={verb.byMargince ? "aiQuiet" : "ghost"}
                onClick={verb.onAct}
              >
                {verb.byMargince && <Sparkles aria-hidden="true" />}
                {verb.label}
              </Button>
            ))}
        </span>
      )}
    </PanelRow>
  );
}
